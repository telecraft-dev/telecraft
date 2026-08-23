package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// The git-delivered collectors' half of prepare (REQ-041, the Foreign
// path).
//
// A served collector's Supervisor configuration is composed: the renderer's
// artefact with the operator's identity merged over it (supervisor.go). A
// git-delivered collector is not composed at all. Its configuration file is
// the rendered artefact itself, mounted straight out of the tree the
// renderer wrote, which is what "delivered by git" means when it is taken
// literally. The operator's half sits beside it as a second file, and the
// collector merges the two for itself.
//
// So this copies rather than merges. Pre-merging them here would hide the
// thing this path exists to show: what runs is the artefact plus whatever
// else is on the box, and nothing the platform sends can overwrite it
// (ADR-0005).

// localFileName is what a git-delivered collector's local file is called
// once written. The compose file mounts it under this name.
const localFileName = "local.yaml"

// writeLocalFile copies one authored local file into a collector's run
// directory, with a header saying where it came from.
//
// A scenario is free to overwrite what this writes: on the Foreign path an
// operator editing the file on the box is the whole mechanism, and the next
// prepare puts the authored one back.
func writeLocalFile(localPath, dir string) (string, error) {
	body, err := os.ReadFile(localPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	header := fmt.Sprintf(""+
		"# Copied by telecraft-devenv from %s.\n"+
		"# Generated: edit that file, not this one.\n"+
		"#\n"+
		"# The local file beside a git-delivered collector's artefact. The\n"+
		"# collector is given both with --config, and merges them itself: the\n"+
		"# Telecraft delivers nothing here, so nothing it could send would\n"+
		"# overwrite what is in this file.\n",
		localPath)
	path := filepath.Join(dir, localFileName)
	if err := os.WriteFile(path, append([]byte(header), body...), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
