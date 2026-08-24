package substrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Name is the file name one artefact version lives under. Naming by ref is
// what lets versions sit side by side: installed versions are retained,
// never replaced (ADR-0020 §9), which is what makes an upgrade-impact diff
// between two of them a cheap thing to compute.
func Name(prefix, ref string) string {
	return prefix + ref + ".json"
}

// Write stores one artefact under dir, named for its ref. The write is
// atomic (the bytes land in a temp file first and are renamed into place),
// so a reader never sees a half-written artefact. If the file already holds
// exactly these bytes the write is skipped and changed is false:
// re-importing the same ref is a no-op, not a rewrite.
func Write(a Artefact, dir, prefix string) (path string, changed bool, err error) {
	ref := a.Version()
	if ref == "" {
		return "", false, fmt.Errorf("the artefact records no ref, so it cannot be named or versioned")
	}
	if strings.ContainsAny(ref, `/\`) {
		return "", false, fmt.Errorf("ref %q cannot name an artefact file", ref)
	}
	encoded, err := a.Encode()
	if err != nil {
		return "", false, err
	}
	path = filepath.Join(dir, Name(prefix, ref))

	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, encoded) {
		return path, false, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	tmp, err := os.CreateTemp(dir, Name(prefix, ref)+".tmp")
	if err != nil {
		return "", false, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(encoded); err != nil {
		tmp.Close()
		return "", false, err
	}
	if err := tmp.Close(); err != nil {
		return "", false, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", false, err
	}
	return path, true, nil
}

// LoadStrict decodes one artefact file into v, failing closed. An artefact
// travels (bundled in a release, downloaded, or carried across an air gap,
// ADR-0020 §5), and a tampered or truncated one silently accepted would
// corrupt every judgement made against it. So an unknown field or trailing
// data is a load error naming the file, and the caller's value is never
// partially populated for use: callers validate on top of this and return
// nil rather than a half-built artefact.
//
// The kind names the document in the message ("catalogue", "schema
// registry"), because a reader of the error is holding a file, not a type.
func LoadStrict(path, kind string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if dec.More() {
		return fmt.Errorf("%s: trailing data after the %s document", path, kind)
	}
	return nil
}
