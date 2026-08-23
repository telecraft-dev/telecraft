package forge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeForge records what was proposed: the seam verified without any
// forge, the ADR-0036 pattern applied to this seam's unit layer.
type fakeForge struct {
	proposed  []Change
	proposal  Proposal
	proposeFn func(Change) (Proposal, error)
}

func (f *fakeForge) Name() string { return "fake forge" }

func (f *fakeForge) Capabilities() Capabilities {
	return Capabilities{Proposals: true, ReviewRouting: true, Annotations: true, VerifiedAttribution: true}
}

func (f *fakeForge) Propose(_ context.Context, change Change) (Proposal, error) {
	f.proposed = append(f.proposed, change)
	if f.proposeFn != nil {
		return f.proposeFn(change)
	}
	return f.proposal, nil
}

func author() Identity {
	return Identity{Name: "Jo Author", Email: "jo@example.com", Handle: "jo-author"}
}

func change() Change {
	return Change{
		Branch: "draft/payments-tier",
		Title:  "Raise the payments tier floor",
		Body:   "The composer draft.",
		Author: author(),
		Files: map[string][]byte{
			"teams/payments/tiers/gold.yaml": []byte("tier: gold\n"),
		},
	}
}

func renderOK(ctx context.Context) (map[string][]byte, error) {
	return map[string][]byte{
		"rendered/payments/gold.yaml": []byte("receivers: {}\n"),
		"CODEOWNERS":                  []byte("# generated\n"),
	}, nil
}

// TestSubmitProposesAuthoredPlusRendered is the issue's first criterion at
// the seam: the proposal carries the authored change and the rendered
// artefacts together (ADR-0028 §1).
func TestSubmitProposesAuthoredPlusRendered(t *testing.T) {
	f := &fakeForge{proposal: Proposal{ID: "7", URL: "https://forge.example/7", Branch: "draft/payments-tier"}}

	got, err := Submit(context.Background(), f, change(), renderOK, Retry{})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "7" || got.URL == "" {
		t.Errorf("proposal = %+v, want the forge's identifier and URL passed through", got)
	}
	if len(f.proposed) != 1 {
		t.Fatalf("forge saw %d proposals, want 1", len(f.proposed))
	}
	sent := f.proposed[0]
	for _, path := range []string{"teams/payments/tiers/gold.yaml", "rendered/payments/gold.yaml", "CODEOWNERS"} {
		if _, ok := sent.Files[path]; !ok {
			t.Errorf("proposed files miss %q; got %v", path, sortedPaths(sent.Files))
		}
	}
	if sent.Message != sent.Title {
		t.Errorf("empty message should default to the title, got %q", sent.Message)
	}
}

// TestSubmitAttributesTheActingHuman: the author rides the change and the
// proposal body names them (ADR-0014).
func TestSubmitAttributesTheActingHuman(t *testing.T) {
	f := &fakeForge{}
	if _, err := Submit(context.Background(), f, change(), renderOK, Retry{}); err != nil {
		t.Fatal(err)
	}
	sent := f.proposed[0]
	if sent.Author != author() {
		t.Errorf("author = %+v, want %+v", sent.Author, author())
	}
	for _, want := range []string{"Jo Author <jo@example.com>", "@jo-author", "The commits attribute this change to them", "The composer draft."} {
		if !strings.Contains(sent.Body, want) {
			t.Errorf("proposal body %q misses %q", sent.Body, want)
		}
	}
}

// TestSubmitRefusesAnonymousChanges: no acting human, no proposal: the
// shared-account failure ADR-0014 exists to prevent.
func TestSubmitRefusesAnonymousChanges(t *testing.T) {
	f := &fakeForge{}
	anon := change()
	anon.Author = Identity{}
	_, err := Submit(context.Background(), f, anon, renderOK, Retry{})
	if err == nil || !strings.Contains(err.Error(), "author name and email") {
		t.Fatalf("err = %v, want a refusal asking for the author name and email", err)
	}
	if len(f.proposed) != 0 {
		t.Error("an unattributable change reached the forge")
	}
}

// TestSubmitFailsClosedOnRefusal: a refused render returns immediately:
// no retry, no proposal (ADR-0028 §3).
func TestSubmitFailsClosedOnRefusal(t *testing.T) {
	f := &fakeForge{}
	calls := 0
	refuse := func(ctx context.Context) (map[string][]byte, error) {
		calls++
		return nil, &Refusal{Reason: "component kafka not in team payments' palette"}
	}

	_, err := Submit(context.Background(), f, change(), refuse, Retry{Attempts: 5, Sleep: func(time.Duration) {}})
	if err == nil {
		t.Fatal("refused render returned no error")
	}
	var refusal *Refusal
	if !errors.As(err, &refusal) {
		t.Errorf("err = %v, want the *Refusal preserved for callers", err)
	}
	if calls != 1 {
		t.Errorf("render ran %d times, want 1: a refusal is red however often it is re-run", calls)
	}
	if len(f.proposed) != 0 {
		t.Error("a refused render still opened a proposal")
	}
}

// TestSubmitRetriesTransientRenderErrors: unavailability is ridden out
// with bounded backoff (ADR-0028 §6).
func TestSubmitRetriesTransientRenderErrors(t *testing.T) {
	f := &fakeForge{}
	calls := 0
	flaky := func(ctx context.Context) (map[string][]byte, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("connection refused")
		}
		return renderOK(ctx)
	}
	var waits []time.Duration
	retry := Retry{Attempts: 3, Backoff: 10 * time.Millisecond, Sleep: func(d time.Duration) { waits = append(waits, d) }}

	if _, err := Submit(context.Background(), f, change(), flaky, retry); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("render ran %d times, want 3", calls)
	}
	if len(waits) != 2 || waits[0] != 10*time.Millisecond || waits[1] != 20*time.Millisecond {
		t.Errorf("waits = %v, want doubling backoff [10ms 20ms]", waits)
	}
	if len(f.proposed) != 1 {
		t.Errorf("forge saw %d proposals, want 1 after the retries succeed", len(f.proposed))
	}
}

// TestSubmitFailsClosedWhenRenderStaysUnavailable: attempts exhausted means
// error and no proposal, never a proposal without a rendered tree.
func TestSubmitFailsClosedWhenRenderStaysUnavailable(t *testing.T) {
	f := &fakeForge{}
	calls := 0
	down := func(ctx context.Context) (map[string][]byte, error) {
		calls++
		return nil, errors.New("connection refused")
	}

	_, err := Submit(context.Background(), f, change(), down, Retry{Attempts: 3, Backoff: time.Millisecond, Sleep: func(time.Duration) {}})
	if err == nil || !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("err = %v, want fail-closed after 3 attempts", err)
	}
	if calls != 3 {
		t.Errorf("render ran %d times, want 3", calls)
	}
	if len(f.proposed) != 0 {
		t.Error("an unrendered change still opened a proposal")
	}
}

// TestSubmitRefusesAuthoredWritesUnderRendered: rendered/ is protected:
// humans never commit there (ADR-0028 §2).
func TestSubmitRefusesAuthoredWritesUnderRendered(t *testing.T) {
	f := &fakeForge{}
	sneaky := change()
	sneaky.Files["rendered/payments/gold.yaml"] = []byte("hand-edited: true\n")

	_, err := Submit(context.Background(), f, sneaky, renderOK, Retry{})
	if err == nil || !strings.Contains(err.Error(), "rendered/") {
		t.Fatalf("err = %v, want a refusal naming the protected tree", err)
	}
	if len(f.proposed) != 0 {
		t.Error("a hand-edit under rendered/ reached the forge")
	}
}

// TestSubmitRefusesProjectionCollision: an authored file at a generated
// path makes the committed file nobody's source.
func TestSubmitRefusesProjectionCollision(t *testing.T) {
	f := &fakeForge{}
	colliding := change()
	colliding.Files["CODEOWNERS"] = []byte("# hand-written\n")

	_, err := Submit(context.Background(), f, colliding, renderOK, Retry{})
	if err == nil || !strings.Contains(err.Error(), "CODEOWNERS") {
		t.Fatalf("err = %v, want a collision refusal naming the path", err)
	}
	if len(f.proposed) != 0 {
		t.Error("a colliding change reached the forge")
	}
}

// TestSubmitPropagatesForgeErrors: the forge failing to open the proposal
// is the caller's to see, never swallowed.
func TestSubmitPropagatesForgeErrors(t *testing.T) {
	f := &fakeForge{proposeFn: func(Change) (Proposal, error) {
		return Proposal{}, errors.New("forge unreachable")
	}}
	_, err := Submit(context.Background(), f, change(), renderOK, Retry{})
	if err == nil || !strings.Contains(err.Error(), "forge unreachable") {
		t.Fatalf("err = %v, want the forge error surfaced", err)
	}
}

// TestSubmitWithoutRendererFailsClosed: no renderer wired is not a licence
// to propose unrendered changes.
func TestSubmitWithoutRendererFailsClosed(t *testing.T) {
	f := &fakeForge{}
	_, err := Submit(context.Background(), f, change(), nil, Retry{})
	if err == nil {
		t.Fatal("nil renderer produced a proposal")
	}
	if len(f.proposed) != 0 {
		t.Error("nil renderer still reached the forge")
	}
}
