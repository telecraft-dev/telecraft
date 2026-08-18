package console

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/telecraft-dev/telecraft/internal/allowlist"
	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/conformance"
	"github.com/telecraft-dev/telecraft/internal/drift"
	"github.com/telecraft-dev/telecraft/internal/expectation"
	"github.com/telecraft-dev/telecraft/internal/inventory"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/serving"
)

// Inputs is everything one snapshot reads. The estate root supplies every
// authored object; the rest names the artefacts and files that travel with
// it (ADR-0020 §5, ADR-0037, ADR-0033).
type Inputs struct {
	// Root is the estate checkout: teams.yaml and the teams/ tree beside
	// the committed rendered/ artefacts (ADR-0027).
	Root string

	// Active is the path of the active Catalogue artefact — the one
	// authoring is judged against.
	Active string

	// Catalogues are every installed Catalogue artefact, the active one
	// included: retained, never replaced (ADR-0020 §9).
	Catalogues []string

	// Library is the requirements library directory (REQ-021).
	Library string

	// Exemptions is the authored waivers directory; empty means none,
	// which is the strictest state there is (ADR-0037).
	Exemptions string

	// EstateFile is the conformance estate: each Service's Effective
	// reading per Environment (ADR-0004's Effective leg).
	EstateFile string

	// ReadingsFile declares the two runtime readings a repository cannot
	// hold — see Readings.
	ReadingsFile string

	// Commit is the estate head the snapshot is taken at (ADR-0013).
	Commit string

	// Repository names the estate, for the demo banner's source link.
	Repository string

	// User is the signed-in user the demo presents.
	User User
}

// Build assembles one snapshot. Every judgement in the result is the return
// value of the package that owns it; this function loads, wires and
// projects, and decides nothing about compliance itself.
//
// It fails closed on everything the product fails closed on, plus one rule
// of its own: the committed rendered/ tree must match a fresh render of the
// authored sources (ADR-0028 §2). A snapshot built over a stale artefact
// tree would show collectors being served config the sources no longer
// describe, which is exactly the lie the recompute invariant exists to
// prevent.
func Build(in Inputs) (Bundle, error) {
	if in.Commit == "" {
		return Bundle{}, fmt.Errorf("no commit — every artefact and every claim carries the SHA it was judged at (ADR-0013, ADR-0038 §4a)")
	}

	tree, err := ownership.LoadTeams(filepath.Join(in.Root, ownership.TeamsFile))
	if err != nil {
		return Bundle{}, err
	}
	active, err := catalogue.Load(in.Active)
	if err != nil {
		return Bundle{}, err
	}
	policy, err := allowlist.Load(in.Root, tree, active)
	if err != nil {
		return Bundle{}, err
	}
	bpEstate, bpFindings, err := blueprint.Load(in.Root)
	if err != nil {
		return Bundle{}, err
	}
	topo, err := renderer.LoadTopology(in.Root)
	if err != nil {
		return Bundle{}, err
	}
	selfTel, err := renderer.LoadSelfTelemetry(in.Root)
	if err != nil {
		return Bundle{}, err
	}
	lib, err := requirements.Load(in.Library)
	if err != nil {
		return Bundle{}, err
	}
	cEstate, err := conformance.LoadEstate(in.EstateFile)
	if err != nil {
		return Bundle{}, err
	}
	readings, err := LoadReadings(in.ReadingsFile)
	if err != nil {
		return Bundle{}, err
	}

	floors := renderer.DefaultFloors()
	rendered, err := renderer.Render(renderer.Inputs{
		Estate:        bpEstate,
		Topology:      topo,
		Policy:        policy,
		Catalogue:     active,
		Tree:          tree,
		Floors:        floors,
		SelfTelemetry: selfTel,
		Commit:        in.Commit,
	})
	if err != nil {
		return Bundle{}, err
	}
	if err := verifyRendered(in.Root, rendered); err != nil {
		return Bundle{}, err
	}

	snapshot, err := serving.LoadSnapshot(in.Root, in.Commit)
	if err != nil {
		return Bundle{}, err
	}

	committed, err := drift.LoadRendered(in.Root)
	if err != nil {
		return Bundle{}, err
	}
	driftReport, err := drift.Detect(drift.Inputs{
		Estate:    bpEstate,
		Topology:  topo,
		Catalogue: active,
		Floors:    floors,
		Library:   lib,
		Rendered:  committed,
	})
	if err != nil {
		return Bundle{}, err
	}

	waivers := conformance.Waivers{Grace: cEstate.Grace}
	if in.Exemptions != "" {
		if waivers.Exemptions, err = conformance.LoadExemptions(in.Exemptions); err != nil {
			return Bundle{}, err
		}
	}
	own := ownershipEstate(tree, topo, bpEstate, lib, waivers.Exemptions)
	waivers.InSubtree = subtreeFunc(own)

	b := builder{
		in:             in,
		tree:           tree,
		active:         active,
		policy:         policy,
		bp:             bpEstate,
		topo:           topo,
		floors:         floors,
		lib:            lib,
		cEstate:        cEstate,
		readings:       readings,
		snapshot:       snapshot,
		drift:          driftReport,
		waivers:        waivers,
		own:            own,
		renderFindings: rendered.Findings,
		bpFindings:     bpFindings,
		now:            readings.AsOf,
	}
	return b.build()
}

// verifyRendered holds the recompute invariant (ADR-0028 §2): rendering is
// a pure function of the authored trees, so the committed tree must equal
// a fresh render. A mismatch names the first offending path — the fix is a
// re-render and a commit, never a snapshot built over it.
func verifyRendered(root string, res renderer.Result) error {
	paths := make([]string, 0, len(res.Artefacts))
	for p := range res.Artefacts {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		on, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf("%s is missing from the committed tree — rendering is a pure function of the sources, so main is always consistent (ADR-0028 §2): re-render and commit", rel)
		}
		if !bytes.Equal(on, res.Artefacts[rel]) {
			return fmt.Errorf("%s differs from a fresh render of the sources — a snapshot over a stale artefact would show collectors served config the sources no longer describe (ADR-0028 §2): re-render and commit", rel)
		}
	}
	return nil
}

// ownershipEstate assembles the routing model from what is already loaded,
// rather than asking the estate to author the same owners twice. Every
// authored object the snapshot can raise a finding about is registered
// with the owner its own file carries (REQ-015, ADR-0016).
func ownershipEstate(tree ownership.Tree, topo renderer.Topology, bp blueprint.Estate,
	lib requirements.Library, exemptions []conformance.Exemption) ownership.Estate {

	est := ownership.Estate{Tree: tree, Objects: map[ownership.Ref]ownership.Object{}}
	add := func(kind ownership.ObjectKind, id, owner string) {
		est.Objects[ownership.Ref{Kind: kind, ID: id}] = ownership.Object{
			Kind: kind, ID: id, Owner: ownership.OwnerID(owner),
		}
	}
	for _, t := range topo.SortedTiers() {
		add(ownership.KindTier, t.ID(), t.Owner)
	}
	for _, s := range topo.Services {
		add(ownership.KindService, s.ID(), s.Owner)
	}
	for _, b := range bp.SortedBlueprints() {
		add(ownership.KindBlueprint, b.ID(), b.Owner)
	}
	for _, c := range bp.Components {
		add(ownership.KindComponent, c.ID(), c.Owner)
	}
	for _, r := range lib.Sorted() {
		add(ownership.KindRequirement, r.ID, r.Owner)
	}
	for _, e := range exemptions {
		add(ownership.KindExemption, e.ID, e.Owner)
	}
	return est
}

// subtreeFunc is the hook a team-scoped Exemption resolves through
// (ADR-0037 §2 over ADR-0017).
func subtreeFunc(own ownership.Estate) func(service, team string) (bool, error) {
	return func(service, team string) (bool, error) {
		subtree, err := own.Tree.Subtree(ownership.TeamID(team))
		if err != nil {
			return false, err
		}
		svc, authored := own.Objects[ownership.Ref{Kind: ownership.KindService, ID: service}]
		if !authored {
			return false, nil
		}
		ownerTeam := own.Tree.Owners[svc.Owner].Team
		for _, id := range subtree {
			if id == ownerTeam {
				return true, nil
			}
		}
		return false, nil
	}
}

// builder carries one snapshot's loaded inputs while the documents are
// projected from them.
type builder struct {
	in       Inputs
	tree     ownership.Tree
	active   *catalogue.Catalogue
	policy   *allowlist.Policy
	bp       blueprint.Estate
	topo     renderer.Topology
	floors   renderer.FloorPolicy
	lib      requirements.Library
	cEstate  conformance.Estate
	readings Readings
	snapshot *serving.Snapshot
	drift    drift.Report
	waivers  conformance.Waivers
	own      ownership.Estate

	renderFindings []renderer.Finding
	bpFindings     []blueprint.Finding

	now time.Time
}

// tierView is one Tier's assembled evidence and verdicts.
type tierView struct {
	tier       renderer.Tier
	class      renderer.ServiceClass
	matched    []CollectorReading
	served     int
	git        int
	population inventory.Population
	popFinds   []inventory.Finding
	expect     expectation.TierResult
	findings   []Finding
	provenance []Provenance
}

func (b *builder) build() (Bundle, error) {
	views, collectors, err := b.readEstate()
	if err != nil {
		return Bundle{}, err
	}
	if err := b.judge(views); err != nil {
		return Bundle{}, err
	}

	cards := make([]CardFace, 0, len(views))
	drawers := map[string]CardDrawer{}
	delivery := map[string]DeliverySplit{}
	selectors := map[string]map[string]string{}
	for _, id := range sortedTierIDs(views) {
		v := views[id]
		cards = append(cards, b.face(v))
		drawers[id] = CardDrawer{
			ContractVersion: ContractVersion,
			Tier:            id,
			Findings:        v.findings,
			Provenance:      v.provenance,
		}
		delivery[id] = DeliverySplit{Served: v.served, Git: v.git}
		if len(v.tier.Selector) > 0 {
			selectors[id] = v.tier.Selector
		}
	}

	catalogues, err := b.catalogues()
	if err != nil {
		return Bundle{}, err
	}

	bundle := Bundle{
		Meta: Meta{
			GeneratedAt: time.Now().UTC(),
			Commit:      b.in.Commit,
			Repository:  b.in.Repository,
			EvaluatedAt: b.now,
		},
		Estate: EstateDoc{
			Me:           b.in.User,
			Environments: b.environments(),
			Teams:        b.teams(),
			Cards:        cards,
			Drawers:      drawers,
			Collectors:   collectors,
			Selectors:    selectors,
			Topology: TopologyDoc{
				Sources:  b.sources(),
				Delivery: delivery,
				Hops:     b.hops(),
				Paths:    b.paths(),
			},
			Services:     b.services(),
			Blueprints:   b.blueprints(),
			Catalogue:    b.components(),
			Owners:       b.owners(),
			AllowLists:   b.allowLists(),
			Grants:       b.grants(),
			Floors:       b.floorTable(),
			Requirements: b.requirements(),
		},
		Catalogues: catalogues,
	}
	return bundle, nil
}

// readEstate plays the declared collector estate through the real selector index: each
// collector is matched exactly as the serving path would match it, so the
// snapshot's populations, ungoverned split and cohort membership are the
// serving decision, not a re-implementation of it (ADR-0007, ADR-0013).
func (b *builder) readEstate() (map[string]*tierView, []CollectorRow, error) {
	views := map[string]*tierView{}
	for _, t := range b.topo.SortedTiers() {
		class, err := b.serviceClass(t)
		if err != nil {
			return nil, nil, err
		}
		views[t.ID()] = &tierView{tier: t, class: class}
	}

	rows := make([]CollectorRow, 0, len(b.readings.Collectors))
	for _, c := range b.readings.Collectors {
		match := b.snapshot.Match(c.Attributes)
		row := CollectorRow{
			ID:          c.ID,
			Environment: c.Attributes["deployment.environment"],
			State:       c.State,
			Version:     c.Version,
			Attributes:  c.Attributes,
		}
		if !c.LastSeen.IsZero() {
			row.LastSeen = c.LastSeen.UTC().Format(time.RFC3339)
		}
		if match.Unmatched {
			// Ungoverned is a concern, never a failure, and no stigma
			// attaches to the delivery path — only to matching no
			// selector (ADR-0031 §1).
			row.Ungoverned = c.Delivery
			if c.Delivery != "served" {
				// Read through the estate provider, never served
				// (ADR-0031 §1).
				row.Ungoverned = "foreign"
			}
			rows = append(rows, row)
			continue
		}
		v := views[match.Tier]
		if v == nil {
			return nil, nil, fmt.Errorf("collector %q matched tier %q, which the topology does not hold", c.ID, match.Tier)
		}
		row.Tier = match.Tier
		row.Team = v.tier.Team
		if row.Environment == "" {
			row.Environment = v.tier.Environment
		}
		v.matched = append(v.matched, c)
		if c.Delivery == "served" {
			v.served++
		} else {
			v.git++
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return views, rows, nil
}

// serviceClass derives the Tier's strictness from traversal, never from a
// hand-maintained field (ADR-0025 §4).
func (b *builder) serviceClass(t renderer.Tier) (renderer.ServiceClass, error) {
	var classes []renderer.ServiceClass
	for _, s := range b.topo.Traversing(t.ID()) {
		if s.Class != "" {
			classes = append(classes, s.Class)
		}
	}
	return b.floors.Strictest(classes)
}
