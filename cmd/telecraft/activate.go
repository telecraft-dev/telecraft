package main

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/telecraft-dev/telecraft/internal/activation"
	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// CataloguesDir and SchemaRegistriesDir are where an estate keeps the
// versions the import pipeline wrote, one directory per substrate.
const (
	CataloguesDir       = "catalogues"
	SchemaRegistriesDir = "schema-registries"
)

// runActivate is the operator's activation route: show what changes, and
// designate the version only when the operator says so (ADR-0020 §6).
//
// Without -confirm it computes the impact report, prints it, and writes
// nothing. That is the whole of "no silent auto-apply" on this path: the
// command an operator runs to look is not the command that changes the
// estate, and the one that changes it takes the report as its argument.
//
// The report a Schema Registry activation shows here is the version diff.
// The estate half, which Services stop passing, needs a reading of landed
// telemetry, and this command takes none: the console is where an operator
// is prompted with both halves. The report says so rather than leaving the
// silence to be read as an all-clear.
//
// Exit codes: 0, the report was computed (and the version activated, with
// -confirm); 1, the activation was refused; 2, the command could not run.
func runActivate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("activate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	estate := fs.String("estate", "", "estate directory holding teams.yaml and the authored objects (required)")
	substrate := fs.String("substrate", "", "which substrate to activate a version of: catalogue or schema-registry (required)")
	version := fs.String("version", "", "the imported version to activate (required)")
	artefacts := fs.String("artefacts", "", "directory of installed artefacts for the substrate (default: the substrate's directory under -estate)")
	by := fs.String("by", "", "the owner deciding the activation, needed with -confirm")
	confirm := fs.Bool("confirm", false, "record the activation, after reading the report")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *estate == "" || *substrate == "" || *version == "" {
		fmt.Fprintln(stderr, "activate: -estate, -substrate and -version are required")
		return 2
	}
	kind, err := substrateKind(*substrate)
	if err != nil {
		fmt.Fprintf(stderr, "activate: %v\n", err)
		return 2
	}
	dir := *artefacts
	if dir == "" {
		dir = filepath.Join(*estate, defaultArtefactDir(kind))
	}

	record, err := activation.Load(*estate)
	if err != nil {
		fmt.Fprintf(stderr, "activate: %v\n", err)
		return 2
	}
	active, _ := record.Active(kind)

	report, err := impactReport(kind, *estate, dir, active, *version)
	if err != nil {
		fmt.Fprintf(stderr, "activate: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, report.Summary())
	for _, line := range report.Lines() {
		fmt.Fprintf(stdout, "  %s\n", line)
	}
	fmt.Fprintln(stdout)

	if !*confirm {
		fmt.Fprintf(stdout, "Nothing has changed. Run the same command with -confirm and -by <owner> to activate %s %s.\n", kind.Name(), *version)
		return 0
	}
	if *by == "" {
		fmt.Fprintln(stderr, "activate: -by is required with -confirm. The record names the owner who decided the activation")
		return 2
	}

	updated, err := activation.Apply(record, kind, report, ownership.OwnerID(*by), time.Now().UTC())
	if err != nil {
		fmt.Fprintf(stderr, "activate: %v\n", err)
		return 1
	}
	if err := activation.Save(*estate, updated); err != nil {
		fmt.Fprintf(stderr, "activate: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "%s %s is active, recorded in %s. Commit it: the review is the audit.\n",
		kind.Name(), *version, filepath.Join(*estate, activation.File))
	return 0
}

func substrateKind(name string) (activation.Kind, error) {
	switch name {
	case "catalogue":
		return activation.Catalogue, nil
	case "schema-registry", "schema_registry":
		return activation.SchemaRegistry, nil
	}
	return "", fmt.Errorf("%q is not a substrate. The substrates are catalogue and schema-registry", name)
}

func defaultArtefactDir(kind activation.Kind) string {
	if kind == activation.SchemaRegistry {
		return SchemaRegistriesDir
	}
	return CataloguesDir
}

// impactReport computes the report for one activation, loading the active
// version beside the candidate. Both are retained by design (ADR-0020 §9),
// which is what makes the diff a cheap thing to compute.
func impactReport(kind activation.Kind, estate, dir, active, candidate string) (activation.Report, error) {
	if kind == activation.SchemaRegistry {
		return schemaRegistryReport(dir, active, candidate)
	}
	return catalogueReport(estate, dir, active, candidate)
}

func catalogueReport(estate, dir, active, candidate string) (activation.Report, error) {
	to, err := catalogue.Load(filepath.Join(dir, catalogue.ArtefactName(candidate)))
	if err != nil {
		return activation.Report{}, err
	}
	var from *catalogue.Catalogue
	if active != "" {
		from, err = catalogue.Load(filepath.Join(dir, catalogue.ArtefactName(active)))
		if err != nil {
			return activation.Report{}, fmt.Errorf("the active version cannot be read, so there is nothing to compare against: %w", err)
		}
	}

	est, _, err := blueprint.Load(estate)
	if err != nil {
		return activation.Report{}, err
	}
	topo, err := renderer.LoadTopology(estate)
	if err != nil {
		return activation.Report{}, err
	}
	tree, err := ownership.LoadTeams(filepath.Join(estate, ownership.TeamsFile))
	if err != nil {
		return activation.Report{}, err
	}
	return activation.CatalogueImpact(activation.CatalogueInputs{
		From:     from,
		To:       to,
		Estate:   est,
		Topology: topo,
		Tree:     tree,
		Floors:   renderer.DefaultFloors(),
	})
}

func schemaRegistryReport(dir, active, candidate string) (activation.Report, error) {
	to, err := schemaregistry.Load(filepath.Join(dir, schemaregistry.ArtefactName(candidate)))
	if err != nil {
		return activation.Report{}, err
	}
	var from *schemaregistry.Registry
	if active != "" {
		from, err = schemaregistry.Load(filepath.Join(dir, schemaregistry.ArtefactName(active)))
		if err != nil {
			return activation.Report{}, fmt.Errorf("the active version cannot be read, so there is nothing to compare against: %w", err)
		}
	}
	return activation.RegistryImpact(activation.RegistryInputs{From: from, To: to})
}
