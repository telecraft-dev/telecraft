package rollout

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/pkg/forge"
)

// AdvanceBranch is the deterministic branch an advance proposal lives on
// (ADR-0029 §8): deterministic names plus the forge's ref compare-and-swap
// mean racing HA replicas converge (the loser sees the branch exists and
// no-ops), so duplicate proposals are structurally impossible and no
// leader is needed. The stage number is the one being advanced to,
// 1-based; the final advance carries the number one past the last stage.
func AdvanceBranch(r renderer.Rollout) string {
	return fmt.Sprintf("telecraft/rollout/%s/%s/advance-%d", r.Team, r.Name, r.Stage+2)
}

// AbortBranch is the deterministic branch an abort proposal lives on,
// named for the stage it aborts from (1-based).
func AbortBranch(r renderer.Rollout) string {
	return fmt.Sprintf("telecraft/rollout/%s/%s/abort-%d", r.Team, r.Name, r.Stage+1)
}

// RolloutFilePath is the Rollout's authored path in the estate layout
// (ADR-0029 §2).
func RolloutFilePath(r renderer.Rollout) string {
	return "teams/" + r.Team + "/rollouts/" + r.Name + ".yaml"
}

// tierFilePath is the target Tier's authored path (ADR-0027).
func tierFilePath(t renderer.Tier) string {
	return "teams/" + t.Team + "/tiers/" + t.Name + ".yaml"
}

// AdvanceChange builds the advance proposal for a stage whose exit
// criteria are met (ADR-0029 §5): a reviewed edit of the Rollout file
// bumping the active stage, or, on the final stage, the completion:
// the Tier flipped to single-bound *to* and the Rollout file deleted,
// which retires the `@next` artefact in the same render. root is the
// estate checkout the authored files are read from.
func AdvanceChange(root string, r renderer.Rollout, tier renderer.Tier, v Verdict, author forge.Identity) (forge.Change, error) {
	if v.Decision != DecisionAdvance {
		return forge.Change{}, fmt.Errorf("verdict is %s, not an advance: only an advance verdict can open an advance proposal", v.Decision)
	}
	body := fmt.Sprintf("Evidence: %s.\n\nTelecraft only proposes this change. A human reviews and merges it.", v.Evidence.Summary())
	change := forge.Change{
		Branch: AdvanceBranch(r),
		Author: author,
		Files:  map[string][]byte{},
	}

	rolloutRaw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(RolloutFilePath(r))))
	if err != nil {
		return forge.Change{}, err
	}
	if r.Final() {
		tierRaw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(tierFilePath(tier))))
		if err != nil {
			return forge.Change{}, err
		}
		rebound, err := setTopLevel(tierRaw, "blueprint", r.To)
		if err != nil {
			return forge.Change{}, fmt.Errorf("%s: %w", tierFilePath(tier), err)
		}
		change.Title = fmt.Sprintf("Complete rollout %s: rebind tier %s to %s", r.ID(), r.Tier, r.To)
		change.Body = body
		change.Files[tierFilePath(tier)] = rebound
		change.Files[RolloutFilePath(r)] = nil // closed: the @next artefact retires with it
		return change, nil
	}

	bumped, err := setTopLevel(rolloutRaw, "stage", r.Stage+1)
	if err != nil {
		return forge.Change{}, fmt.Errorf("%s: %w", RolloutFilePath(r), err)
	}
	change.Title = fmt.Sprintf("Advance rollout %s to stage %d of %d", r.ID(), r.Stage+2, len(r.Stages))
	change.Body = body
	change.Files[RolloutFilePath(r)] = bumped
	return change, nil
}

// AbortChange builds the abort proposal (ADR-0029 §6): deleting the
// Rollout file reverts the Tier to single-bound *from* (the Tier's own
// binding never moved) and retires the `@next` artefact. Collectors that
// individually broke have already self-reverted (ADR-0010); this closes
// the book.
func AbortChange(root string, r renderer.Rollout, v Verdict, author forge.Identity) (forge.Change, error) {
	if v.Decision != DecisionAbort {
		return forge.Change{}, fmt.Errorf("verdict is %s, not an abort: only an abort verdict can open an abort proposal", v.Decision)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(RolloutFilePath(r)))); err != nil {
		return forge.Change{}, err
	}
	return forge.Change{
		Branch: AbortBranch(r),
		Title:  fmt.Sprintf("Abort rollout %s: revert tier %s to single-bound %s", r.ID(), r.Tier, r.From),
		Body: fmt.Sprintf("%s\n\nEvidence: %s.\n\nDeleting the Rollout file returns the Tier to the previous version and retires the @next artefact.",
			v.Reason, v.Evidence.Summary()),
		Author: author,
		Files:  map[string][]byte{RolloutFilePath(r): nil},
	}, nil
}

// Propose acts on a verdict through the forge: the advance or abort
// proposal goes through the render-in-PR submission flow (forge.Submit,
// ADR-0028 §1), so the proposal that opens carries the refreshed rendered
// tree beside the authored edit. Any other verdict proposes nothing and
// returns proposed false, because the passive halt has no active step (ADR-0029
// §6). The author attributes the proposal (ADR-0014): the advance
// executes the Rollout owner's authored staging intent, and the commits
// name whoever the platform is configured to act as.
func Propose(ctx context.Context, f forge.Forge, render forge.RenderFunc, retry forge.Retry, root string, r renderer.Rollout, tier renderer.Tier, v Verdict, author forge.Identity) (forge.Proposal, bool, error) {
	var change forge.Change
	var err error
	switch v.Decision {
	case DecisionAdvance:
		change, err = AdvanceChange(root, r, tier, v, author)
	case DecisionAbort:
		change, err = AbortChange(root, r, v, author)
	default:
		return forge.Proposal{}, false, nil
	}
	if err != nil {
		return forge.Proposal{}, false, err
	}
	p, err := forge.Submit(ctx, f, change, render, retry)
	if err != nil {
		return forge.Proposal{}, true, err
	}
	return p, true, nil
}

// setTopLevel replaces (or appends) one top-level key's value in an
// authored YAML document, preserving the rest of the document, comments
// included, through a node round-trip: the platform edits one field of a
// human-owned file, never rewrites it.
func setTopLevel(raw []byte, key string, value any) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("the file does not hold one mapping document")
	}
	encoded := &yaml.Node{}
	if err := encoded.Encode(value); err != nil {
		return nil, err
	}
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = encoded
			return marshalIndented(&doc)
		}
	}
	keyNode := &yaml.Node{}
	keyNode.SetString(key)
	m.Content = append(m.Content, keyNode, encoded)
	return marshalIndented(&doc)
}

func marshalIndented(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
