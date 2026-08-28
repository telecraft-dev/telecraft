package instance

import (
	"net/url"

	"github.com/telecraft-dev/telecraft/internal/console"
)

// The read endpoints, each a projection of the documents the console
// package computed. Nothing here judges anything: every value is carried
// from the document set, and the shapes are the ones console/README.md
// documents and console/tools/fixture-backend.mjs implements.

// indexedObject is one entry of the jump-to-object index.
type indexedObject struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Team        string `json:"team,omitempty"`
	Environment string `json:"environment,omitempty"`
}

func objectsDoc(b *console.Bundle, _ url.Values) (any, bool) {
	out := []indexedObject{}
	var walk func(console.TeamNode)
	walk = func(team console.TeamNode) {
		out = append(out, indexedObject{Kind: "team", ID: team.ID, Name: team.Name, Team: team.ID})
		for _, child := range team.Teams {
			walk(child)
		}
	}
	walk(b.Estate.Teams)
	for _, card := range b.Estate.Cards {
		out = append(out, indexedObject{
			Kind: "tier", ID: card.Tier, Name: card.Name,
			Team: card.Team, Environment: card.Environment,
		})
	}
	for _, service := range b.Estate.Services {
		out = append(out, indexedObject{Kind: "service", ID: service.ID, Name: service.Name, Team: service.Team})
	}
	for _, bp := range b.Estate.Blueprints {
		out = append(out, indexedObject{Kind: "blueprint", ID: bp.ID, Name: bp.Name, Team: bp.Team})
	}
	for _, component := range b.Estate.Catalogue {
		out = append(out, indexedObject{Kind: "component", ID: component.ID, Name: component.Name, Team: component.Team})
	}
	for _, r := range b.Estate.Rollouts {
		out = append(out, indexedObject{Kind: "rollout", ID: r.ID, Name: r.Name, Team: r.Team})
	}
	// Catalogue entries are browsable and deep-linkable though nobody owns
	// them, so they carry no team.
	for _, entry := range activeEntries(b) {
		name := entry.DisplayName
		if name == "" {
			name = entry.Type
		}
		out = append(out, indexedObject{Kind: "entry", ID: entry.Class + "/" + entry.Type, Name: name})
	}
	return out, true
}

// ungovernedCounts is the dedicated band's split: collectors matching no
// Tier selector, by how they are read. Concern, never failure, and in no
// compliance denominator.
type ungovernedCounts struct {
	Served  int `json:"served"`
	Foreign int `json:"foreign"`
}

type estatePayload struct {
	Environments []string               `json:"environments"`
	Teams        console.TeamNode       `json:"teams"`
	Cards        []console.CardFace     `json:"cards"`
	Ungoverned   ungovernedCounts       `json:"ungoverned"`
	Settings     console.EstateSettings `json:"settings"`
}

func estateDoc(b *console.Bundle, _ url.Values) (any, bool) {
	var ungoverned ungovernedCounts
	for _, row := range b.Estate.Collectors {
		switch row.Ungoverned {
		case "served":
			ungoverned.Served++
		case "foreign":
			ungoverned.Foreign++
		}
	}
	cards := b.Estate.Cards
	if cards == nil {
		cards = []console.CardFace{}
	}
	environments := b.Estate.Environments
	if environments == nil {
		environments = []string{}
	}
	return estatePayload{
		Environments: environments,
		Teams:        b.Estate.Teams,
		Cards:        cards,
		Ungoverned:   ungoverned,
		Settings:     b.Estate.Settings,
	}, true
}

func drawerDoc(b *console.Bundle, q url.Values) (any, bool) {
	tier := q.Get("tier")
	if drawer, ok := b.Estate.Drawers[tier]; ok {
		return drawer, true
	}
	// A Tier with nothing to report answers empty, honestly.
	return console.CardDrawer{ContractVersion: console.ContractVersion, Tier: tier}, true
}

func collectorsDoc(b *console.Bundle, _ url.Values) (any, bool) {
	if b.Estate.Collectors == nil {
		return []console.CollectorRow{}, true
	}
	return b.Estate.Collectors, true
}

// topologyTier is one Tier at authored grain, with the selector-matched
// count the card face carries and the Tier-aggregated delivery split.
type topologyTier struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Team         string                `json:"team"`
	Environment  string                `json:"environment"`
	ServiceClass string                `json:"serviceClass,omitempty"`
	Matched      int                   `json:"matched"`
	Delivery     console.DeliverySplit `json:"delivery"`
}

type topologyPayload struct {
	Environments []string                 `json:"environments"`
	Tiers        []topologyTier           `json:"tiers"`
	Sources      []console.TopologySource `json:"sources"`
	Hops         []console.TopologyHop    `json:"hops"`
	Paths        []console.TopologyPath   `json:"paths"`
}

func topologyDoc(b *console.Bundle, _ url.Values) (any, bool) {
	tiers := make([]topologyTier, 0, len(b.Estate.Cards))
	for _, card := range b.Estate.Cards {
		delivery, ok := b.Estate.Topology.Delivery[card.Tier]
		if !ok {
			delivery = console.DeliverySplit{Served: card.Population.Matched}
		}
		tiers = append(tiers, topologyTier{
			ID: card.Tier, Name: card.Name, Team: card.Team,
			Environment:  card.Environment,
			ServiceClass: card.ServiceClass,
			Matched:      card.Population.Matched,
			Delivery:     delivery,
		})
	}
	return topologyPayload{
		Environments: orEmpty(b.Estate.Environments),
		Tiers:        tiers,
		Sources:      orEmpty(b.Estate.Topology.Sources),
		Hops:         orEmpty(b.Estate.Topology.Hops),
		Paths:        orEmpty(b.Estate.Topology.Paths),
	}, true
}

func rolloutsDoc(b *console.Bundle, _ url.Values) (any, bool) {
	return orEmpty(b.Estate.Rollouts), true
}

func blueprintsDoc(b *console.Bundle, _ url.Values) (any, bool) {
	return orEmpty(b.Estate.Blueprints), true
}

func catalogueDoc(b *console.Bundle, _ url.Values) (any, bool) {
	return orEmpty(b.Estate.Catalogue), true
}

// catalogueVersionSummary is one installed catalogue in the versions list:
// retained, never replaced, with the one designated active.
type catalogueVersionSummary struct {
	Version    string                  `json:"version"`
	Active     bool                    `json:"active"`
	Components int                     `json:"components"`
	Source     console.CatalogueSource `json:"source"`
}

type catalogueVersionsPayload struct {
	Active   string                    `json:"active"`
	Versions []catalogueVersionSummary `json:"versions"`
}

func catalogueVersionsDoc(b *console.Bundle, _ url.Values) (any, bool) {
	versions := make([]catalogueVersionSummary, 0, len(b.Catalogues.Versions))
	for _, v := range b.Catalogues.Versions {
		versions = append(versions, catalogueVersionSummary{
			Version:    v.Version,
			Active:     v.Version == b.Catalogues.Active,
			Components: len(v.Components),
			Source:     v.Source,
		})
	}
	return catalogueVersionsPayload{Active: b.Catalogues.Active, Versions: versions}, true
}

func catalogueEntriesDoc(b *console.Bundle, q url.Values) (any, bool) {
	version := q.Get("version")
	if version == "" {
		version = b.Catalogues.Active
	}
	for _, v := range b.Catalogues.Versions {
		if v.Version == version {
			return orEmpty(v.Components), true
		}
	}
	return nil, false
}

func activationsDoc(b *console.Bundle, _ url.Values) (any, bool) {
	return b.Activations, true
}

type governancePayload struct {
	Owners     []console.OwnerDoc     `json:"owners"`
	AllowLists []console.AllowListDoc `json:"allowLists"`
	Grants     []console.GrantDoc     `json:"grants"`
}

func governanceDoc(b *console.Bundle, _ url.Values) (any, bool) {
	return governancePayload{
		Owners:     orEmpty(b.Estate.Owners),
		AllowLists: orEmpty(b.Estate.AllowLists),
		Grants:     orEmpty(b.Estate.Grants),
	}, true
}

// endorsementsDoc answers the Endorsement ledger. The document computation
// carries none yet, so the ledger is empty here on every estate.
func endorsementsDoc(_ *console.Bundle, _ url.Values) (any, bool) {
	return []struct{}{}, true
}

// activeEntries is the active catalogue's entries: what authoring is judged
// against.
func activeEntries(b *console.Bundle) []console.CatalogueEntryDoc {
	for _, v := range b.Catalogues.Versions {
		if v.Version == b.Catalogues.Active {
			return v.Components
		}
	}
	return nil
}

// orEmpty turns a nil slice into an empty one, because the contract
// promises a list: a reader takes its length without first asking whether
// the list is there.
func orEmpty[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}
