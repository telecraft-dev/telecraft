package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"sort"
)

type result struct {
	executables []string
	checked     int
}

// check is the entry point used by main. It asks git for the list of tracked
// files and delegates to checkPaths for the actual inspection.
func check(root string) (result, error) {
	paths, err := lsFiles(root)
	if err != nil {
		return result{}, err
	}
	return checkPaths(os.DirFS(root), paths)
}

// lsFiles returns every path currently tracked by git in root. Using git's own
// list means gitignored files (devenv/run/, build artefacts, node_modules, and
// so on) are never inspected, and there is no need to enumerate what to skip.
func lsFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var paths []string
	for _, entry := range bytes.Split(out, []byte{0}) {
		if len(entry) > 0 {
			paths = append(paths, string(entry))
		}
	}
	return paths, nil
}

// checkPaths inspects each path in fsys and collects those whose first bytes
// match a compiled executable format. It is separate from check so that tests
// can supply an arbitrary file list without needing a live git repository.
func checkPaths(fsys fs.FS, paths []string) (result, error) {
	r := result{checked: len(paths)}
	for _, path := range paths {
		exe, err := isExecutable(fsys, path)
		if err != nil {
			return result{}, fmt.Errorf("%s: %w", path, err)
		}
		if exe {
			r.executables = append(r.executables, path)
		}
	}
	sort.Strings(r.executables)
	return r, nil
}

// isExecutable reads the first four bytes of path inside fsys and reports
// whether they match the magic of a known compiled executable format.
func isExecutable(fsys fs.FS, path string) (bool, error) {
	f, err := fsys.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	var header [4]byte
	n, err := io.ReadAtLeast(f, header[:], 1)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		// Shorter than 4 bytes; cannot be a compiled executable.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return matchesMagic(header[:n]), nil
}

// matchesMagic reports whether h begins with the magic bytes of a Mach-O
// (including fat binary), ELF, or PE executable.
//
// 0xCAFEBABE is also the class-file magic for the JVM. Nothing in this
// repository targets the JVM, so that is not a concern here, but the overlap
// means the flag is not a bug.
func matchesMagic(h []byte) bool {
	if len(h) < 2 {
		return false
	}
	// PE (Windows): MZ header.
	if h[0] == 'M' && h[1] == 'Z' {
		return true
	}
	if len(h) < 4 {
		return false
	}
	// ELF (Linux, most Unix).
	if h[0] == 0x7f && h[1] == 'E' && h[2] == 'L' && h[3] == 'F' {
		return true
	}
	// Mach-O little-endian 32-bit.
	if h[0] == 0xce && h[1] == 0xfa && h[2] == 0xed && h[3] == 0xfe {
		return true
	}
	// Mach-O little-endian 64-bit.
	if h[0] == 0xcf && h[1] == 0xfa && h[2] == 0xed && h[3] == 0xfe {
		return true
	}
	// Mach-O big-endian 32-bit.
	if h[0] == 0xfe && h[1] == 0xed && h[2] == 0xfa && h[3] == 0xce {
		return true
	}
	// Mach-O big-endian 64-bit.
	if h[0] == 0xfe && h[1] == 0xed && h[2] == 0xfa && h[3] == 0xcf {
		return true
	}
	// Mach-O fat binary (and Java .class — see matchesMagic's doc comment).
	if h[0] == 0xca && h[1] == 0xfe && h[2] == 0xba && h[3] == 0xbe {
		return true
	}
	return false
}
