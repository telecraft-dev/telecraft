package activation

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Resolution is which version of a substrate judges one subject, and how
// well the answer is known.
//
// It exists because designation is not one field (ADR-0020 §9). Authoring
// and the Palette judge against the version an operator designated active,
// and evaluation of a collector consults the Catalogue for the version that
// collector actually runs, which is a discovered fact rather than a
// designated one: the platform does not control collector binaries
// (ADR-0002). One record answers both questions, and this type is the second
// answer.
type Resolution struct {
	// Version is the version judging the subject, empty when nothing can.
	Version string

	// Known is false when no installed version can judge the subject. It is
	// the ADR-0004 flag, meaning what it means everywhere: not knowing is a
	// normal state, and a judgement nobody could make is never reported as
	// a pass or a failure.
	Known bool

	// Degraded is true when the subject is judged against an older version
	// than it runs, which is the "cannot fully know" case ADR-0020 §9 asks
	// for rather than a silent assertion.
	Degraded bool

	// Reason says what happened, for a reader of the surface. It is empty
	// when the answer is exact.
	Reason string
}

// Judge answers which installed Catalogue version judges a collector running
// the given version (ADR-0020 §9).
//
// An exact match is the ordinary answer. Where the version a collector runs
// has no installed Catalogue, the nearest older installed version judges it
// and the judgement says it is degraded: an older Catalogue can describe a
// component the collector still has, and cannot describe one added since, so
// what it says is true as far as it goes and never further. Where there is
// no older version to fall back on, nothing is known, and the fix is to
// import the missing version.
func Judge(version string, installed []string) Resolution {
	if version == "" {
		return Resolution{Reason: "This collector has not reported which version it runs, so no Catalogue can judge it."}
	}
	for _, v := range installed {
		if v == version {
			return Resolution{Version: version, Known: true}
		}
	}

	nearest, ok := nearestOlder(version, installed)
	if !ok {
		return Resolution{Reason: fmt.Sprintf("No installed Catalogue covers %s or anything older, so this collector's components cannot be judged. Import the Catalogue for %s.", version, version)}
	}
	return Resolution{
		Version:  nearest,
		Known:    true,
		Degraded: true,
		Reason: fmt.Sprintf("No Catalogue is installed for %s, so this collector is judged against %s. Anything added since %s is unknown here. Import the Catalogue for %s.",
			version, nearest, nearest, version),
	}
}

// nearestOlder returns the highest installed version below the one asked
// for. A version neither side can order is skipped rather than guessed at:
// judging a collector against a Catalogue chosen by string comparison would
// be a silent assertion about which one is older.
func nearestOlder(version string, installed []string) (string, bool) {
	want, ok := parseVersion(version)
	if !ok {
		return "", false
	}
	candidates := make([]string, 0, len(installed))
	for _, v := range installed {
		got, ok := parseVersion(v)
		if !ok || !less(got, want) {
			continue
		}
		candidates = append(candidates, v)
	}
	if len(candidates) == 0 {
		return "", false
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, _ := parseVersion(candidates[i])
		b, _ := parseVersion(candidates[j])
		return less(a, b)
	})
	return candidates[len(candidates)-1], true
}

// version is a collector release tag split into its three numbers. Catalogue
// versions are collector release tags and nothing else (ADR-0020 §7), so the
// form is the upstream one: an optional leading v, then major.minor.patch.
type version struct{ major, minor, patch int }

func parseVersion(s string) (version, bool) {
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return version{}, false
	}
	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return version{}, false
		}
		out[i] = n
	}
	return version{out[0], out[1], out[2]}, true
}

func less(a, b version) bool {
	if a.major != b.major {
		return a.major < b.major
	}
	if a.minor != b.minor {
		return a.minor < b.minor
	}
	return a.patch < b.patch
}
