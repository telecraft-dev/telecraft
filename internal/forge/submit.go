package forge

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RenderFunc produces the rendered artefacts a change implies — the call
// the render-in-PR bot makes on every push (ADR-0028 §1). The returned map
// is repository-relative paths to full contents, exactly the shape
// Change.Files takes.
//
// Two failure modes are distinct and must stay so (ADR-0028 §6): a
// *Refusal means the render refused the change — mechanical invalidity or
// the one allow-list hard block (ADR-0022 §3) — and no amount of retrying
// changes the answer; any other error is the renderer being unavailable,
// which retry with backoff may ride out.
type RenderFunc func(ctx context.Context) (map[string][]byte, error)

// Refusal is a render that refused: the rendered tree cannot be produced
// from this change, so the proposal must not open — block-at-render is
// block-at-merge (ADR-0028 §3).
type Refusal struct {
	Reason string
}

func (r *Refusal) Error() string {
	return "render refused: " + r.Reason
}

// Retry bounds the render attempts (ADR-0028 §6: fail closed, with bounded
// retry). Zero values take the defaults.
type Retry struct {
	// Attempts is the total number of render attempts, including the
	// first. Default 3.
	Attempts int

	// Backoff is the wait before the second attempt; it doubles per
	// attempt. Default 2s.
	Backoff time.Duration

	// Sleep replaces the wait in tests. Nil means a context-aware sleep.
	Sleep func(time.Duration)
}

const (
	defaultAttempts = 3
	defaultBackoff  = 2 * time.Second
)

// Submit is the render-in-PR submission flow (ADR-0028 §1): render the
// change fail-closed, merge the rendered artefacts into it, stamp the
// attribution footer, and propose it through the forge. The proposal that
// opens carries the authored change plus the bot-refreshed rendered diffs —
// reviewers judge the blast radius, never approve blind.
//
// Fail closed means no proposal without a rendered tree: a *Refusal from
// render returns immediately — a refused render is red however often it is
// re-run; any other render error is retried with bounded backoff, and when
// the renderer stays unavailable Submit returns the error and proposes
// nothing (ADR-0028 §6). Retry across calls is the branch: proposing the
// same Change.Branch again refreshes the existing proposal, so a fixed
// change turns its check green in place.
func Submit(ctx context.Context, f Forge, change Change, render RenderFunc, retry Retry) (Proposal, error) {
	if err := validate(change); err != nil {
		return Proposal{}, err
	}

	rendered, err := renderClosed(ctx, render, retry)
	if err != nil {
		return Proposal{}, err
	}

	merged, err := merge(change.Files, rendered)
	if err != nil {
		return Proposal{}, err
	}
	change.Files = merged
	change.Body = withAttribution(change.Body, change.Author, f.Name())
	if change.Message == "" {
		change.Message = change.Title
	}

	return f.Propose(ctx, change)
}

func validate(change Change) error {
	if change.Author.Name == "" || change.Author.Email == "" {
		// An unattributable change is the shared-account failure ADR-0014
		// exists to prevent — refused here, not defaulted to a bot.
		return errors.New("submit: change carries no acting human (ADR-0014): author name and email are required")
	}
	if change.Branch == "" {
		return errors.New("submit: change names no branch")
	}
	if change.Title == "" {
		return errors.New("submit: change carries no title")
	}
	for _, path := range sortedPaths(change.Files) {
		if path == "rendered" || strings.HasPrefix(path, "rendered/") {
			return fmt.Errorf("submit: authored file %q sits under the protected rendered/ tree — humans never commit there (ADR-0028 §2)", path)
		}
	}
	return nil
}

// renderClosed runs the render with bounded backoff. A *Refusal fails
// immediately; exhausting the attempts fails closed with the last error —
// the caller gets no proposal either way (ADR-0028 §6).
func renderClosed(ctx context.Context, render RenderFunc, retry Retry) (map[string][]byte, error) {
	if render == nil {
		return nil, errors.New("submit: no renderer wired — a proposal without a rendered tree would break the main-is-always-consistent invariant (ADR-0028 §2)")
	}
	attempts := retry.Attempts
	if attempts <= 0 {
		attempts = defaultAttempts
	}
	backoff := retry.Backoff
	if backoff <= 0 {
		backoff = defaultBackoff
	}

	var last error
	for attempt := 1; attempt <= attempts; attempt++ {
		rendered, err := render(ctx)
		if err == nil {
			return rendered, nil
		}
		var refusal *Refusal
		if errors.As(err, &refusal) {
			return nil, fmt.Errorf("submit: fail closed, no proposal (ADR-0028 §3): %w", err)
		}
		last = err
		if attempt == attempts || ctx.Err() != nil {
			break
		}
		wait(ctx, backoff, retry.Sleep)
		backoff *= 2
	}
	return nil, fmt.Errorf("submit: render unavailable after %d attempts, fail closed, no proposal (ADR-0028 §6): %w", attempts, last)
}

func wait(ctx context.Context, d time.Duration, sleep func(time.Duration)) {
	if sleep != nil {
		sleep(d)
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// merge lays the rendered artefacts over the authored files. A collision
// is refused: a generated projection with an authored double would make
// the committed file nobody's source (ADR-0028 §4 — projections are
// caches, humans edit the sources).
func merge(authored, rendered map[string][]byte) (map[string][]byte, error) {
	out := make(map[string][]byte, len(authored)+len(rendered))
	for path, content := range authored {
		out[path] = content
	}
	for _, path := range sortedPaths(rendered) {
		if _, clash := out[path]; clash {
			return nil, fmt.Errorf("submit: authored file %q collides with a generated projection — the generated file is a cache, edit its source instead (ADR-0028 §4)", path)
		}
		out[path] = rendered[path]
	}
	return out, nil
}

// withAttribution appends the acting-human footer to the proposal body:
// the proposal is opened by the platform's bot identity, so the body names
// the human the commits are authored as (ADR-0014).
func withAttribution(body string, author Identity, forgeName string) string {
	who := fmt.Sprintf("%s <%s>", author.Name, author.Email)
	if author.Handle != "" {
		who = fmt.Sprintf("%s (@%s)", who, author.Handle)
	}
	footer := fmt.Sprintf("Proposed by %s via %s; the commits carry this authorship (ADR-0014).", who, forgeName)
	if body == "" {
		return footer
	}
	return body + "\n\n" + footer
}

func sortedPaths(files map[string][]byte) []string {
	out := make([]string, 0, len(files))
	for path := range files {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}
