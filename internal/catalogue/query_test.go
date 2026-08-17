package catalogue

import (
	"sort"
	"testing"
)

// The Catalogue is queryable in the terms downstream consumers judge in:
// by (class, type) key, by class, by signal, by stability level.
func TestQueryAPI(t *testing.T) {
	cat, _, err := Import(snapshotDir, snapshotSource)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("lookup misses honestly", func(t *testing.T) {
		if _, ok := cat.Lookup(Receiver, "no_such_thing"); ok {
			t.Error("a type that was never imported resolved")
		}
		// kafka exists as a receiver and an exporter — never as a connector.
		// A class-blind lookup would wrongly resolve this.
		if _, ok := cat.Lookup(Connector, "kafka"); ok {
			t.Error("kafka resolved under a class it does not exist in")
		}
	})

	t.Run("by class, in type order", func(t *testing.T) {
		exts := cat.ByClass(Extension)
		if len(exts) != 3 {
			t.Fatalf("got %d extensions, want 3", len(exts))
		}
		if !sort.SliceIsSorted(exts, func(i, j int) bool { return exts[i].Type < exts[j].Type }) {
			t.Error("ByClass is not in type order")
		}
		for _, e := range exts {
			if e.Class != Extension {
				t.Errorf("ByClass(extension) returned a %s", e.Class)
			}
		}
	})

	t.Run("by signal", func(t *testing.T) {
		// Exactly the two kafka components carry profiles (in development).
		profiled := cat.SupportingSignal("profiles")
		if len(profiled) != 2 {
			t.Fatalf("got %d components supporting profiles, want 2: %+v", len(profiled), profiled)
		}
		for _, c := range profiled {
			if c.Type != "kafka" {
				t.Errorf("unexpected profiles support on %s/%s", c.Class, c.Type)
			}
		}
		// The connector's signal vocabulary token passes through verbatim.
		if got := cat.SupportingSignal("traces_to_metrics"); len(got) != 1 || got[0].Type != "span_metrics" {
			t.Errorf("traces_to_metrics = %+v, want span_metrics alone", got)
		}
	})

	t.Run("by stability", func(t *testing.T) {
		if got := cat.WithStability(Deprecated); len(got) != 1 || got[0].Type != "mezmo" {
			t.Errorf("WithStability(deprecated) = %+v, want mezmo alone", got)
		}
		if got := cat.WithStability(Unmaintained); len(got) != 0 {
			t.Errorf("WithStability(unmaintained) = %+v, want none", got)
		}
	})

	t.Run("signals are stable and sorted", func(t *testing.T) {
		kafka, _ := cat.Lookup(Receiver, "kafka")
		want := []string{"logs", "metrics", "profiles", "traces"}
		got := kafka.Signals()
		if len(got) != len(want) {
			t.Fatalf("signals = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("signals = %v, want %v", got, want)
			}
		}
	})

	t.Run("version is the pinned tag", func(t *testing.T) {
		if cat.Version() != "v0.158.0" {
			t.Errorf("Version() = %q, want the pinned tag", cat.Version())
		}
	})
}
