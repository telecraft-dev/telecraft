package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// headerBytes is how much of each file the check reads. It is generous
// because a PE file puts the header that identifies it at an offset the
// first 64 bytes name, and that offset is free to be large; it is bounded
// because the answer never depends on the rest of the file.
const headerBytes = 4096

// Finding names one tracked file that is a compiled executable, and the
// format it declares itself to be.
type Finding struct {
	Path   string
	Format string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s: a compiled %s executable is tracked (issue #122)", f.Path, f.Format)
}

type Result struct {
	Findings []Finding
	Scanned  int
}

// Run reports every tracked file under root that is a compiled executable.
//
// The scope is what git tracks rather than what the directory holds: a
// build artefact sitting in a working tree is what the tools are for, and
// only the ones that reach the index reach a clone.
func Run(root string) (Result, error) {
	paths, err := trackedFiles(root)
	if err != nil {
		return Result{}, err
	}
	return scan(root, paths)
}

// trackedFiles asks git for the index rather than walking the tree, so the
// check inherits .gitignore for free and stays honest about what a clone
// receives. Paths are NUL-separated because a filename may contain
// anything else.
func trackedFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("listing the tracked files of %s: %s", root, detail)
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// scan classifies each path, which is slash-separated and relative to root.
func scan(root string, paths []string) (Result, error) {
	var result Result
	for _, p := range paths {
		header, ok, err := readHeader(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			return Result{}, err
		}
		if !ok {
			// A submodule, a symbolic link, or a path staged for deletion.
			// None of them is a file this check can read, and none of them
			// carries bytes into a clone.
			continue
		}
		result.Scanned++
		if format := executableFormat(header); format != "" {
			result.Findings = append(result.Findings, Finding{Path: p, Format: format})
		}
	}
	return result, nil
}

// readHeader returns the opening bytes of a regular file. The second return
// value is false when the path is not a regular file that exists, which is
// not an error: git tracks a few things that are not files.
func readHeader(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	// A short file, and an empty one, reads what is there and reports EOF.
	// Either way it was read, and what it declares is decided by the bytes
	// that came back.
	buf := make([]byte, headerBytes)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	return buf[:n], true, nil
}

// executableFormat names the compiled-executable format the header
// declares, or the empty string when it declares none.
func executableFormat(header []byte) string {
	switch {
	case isELF(header):
		return "ELF"
	case isMachO(header):
		return "Mach-O"
	case isPE(header):
		return "PE"
	default:
		return ""
	}
}

// isELF matches the four bytes every ELF object opens with. No text format
// begins with a DEL byte, so the magic alone is enough.
func isELF(header []byte) bool {
	return bytes.HasPrefix(header, []byte{0x7f, 'E', 'L', 'F'})
}

// machOMagics are the four magics a single-architecture Mach-O object
// opens with: 32- and 64-bit, each in both byte orders.
var machOMagics = map[uint32]bool{
	0xfeedface: true, // 32-bit
	0xcefaedfe: true, // 32-bit, byte-swapped
	0xfeedfacf: true, // 64-bit
	0xcffaedfe: true, // 64-bit, byte-swapped
}

// machOCPUTypes are the architectures a fat Mach-O binary can name. The set
// is what distinguishes one from a Java class file; see isMachO.
var machOCPUTypes = map[uint32]bool{
	7:          true, // x86
	0x01000007: true, // x86-64
	12:         true, // ARM
	0x0100000c: true, // ARM64
	18:         true, // PowerPC
	0x01000012: true, // PowerPC 64
}

// isMachO matches a Mach-O object, single-architecture or fat.
//
// The fat case needs care. A fat binary opens with 0xcafebabe, which is
// also the first four bytes of every Java class file, so the magic alone
// would flag a fixture that is not an executable at all. What follows the
// magic tells them apart: in a fat binary it is the number of architectures
// and then a table of them, and in a class file it is a version pair whose
// value is at least 45. So the count has to be small and the first
// architecture has to be one that exists.
func isMachO(header []byte) bool {
	if len(header) < 4 {
		return false
	}
	if machOMagics[binary.BigEndian.Uint32(header)] {
		return true
	}

	const fatHeaderBytes = 12 // magic, count, and the first architecture
	if len(header) < fatHeaderBytes {
		return false
	}
	switch binary.BigEndian.Uint32(header) {
	case 0xcafebabe, 0xcafebabf: // 32- and 64-bit architecture tables
	default:
		return false
	}
	if count := binary.BigEndian.Uint32(header[4:]); count == 0 || count > 32 {
		return false
	}
	return machOCPUTypes[binary.BigEndian.Uint32(header[8:])]
}

// isPE matches a Windows executable.
//
// "MZ" opens every PE file, but it opens a DOS stub rather than the PE
// header, and two letters are not evidence: a text file may begin with
// them. The stub carries the offset of the real header at 0x3c, and the
// file is a PE only when the signature is actually there.
func isPE(header []byte) bool {
	const signatureOffset = 0x3c
	if len(header) < signatureOffset+4 || header[0] != 'M' || header[1] != 'Z' {
		return false
	}
	at := int(binary.LittleEndian.Uint32(header[signatureOffset:]))
	if at < signatureOffset+4 || at+4 > len(header) {
		return false
	}
	return bytes.Equal(header[at:at+4], []byte{'P', 'E', 0, 0})
}
