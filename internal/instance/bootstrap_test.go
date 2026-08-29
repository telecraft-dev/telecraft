package instance

import (
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// The address is the boundary (ADR-0082 §2), so it is the thing worth a
// table: everything a reader might type, and what it is allowed to do.
func TestLoopbackAddr(t *testing.T) {
	for _, tc := range []struct {
		endpoint string
		want     bool
	}{
		{"127.0.0.1:4321", true},
		{"localhost:4321", true},
		{"[::1]:4321", true},
		{"127.0.0.2:4321", true},
		// Every interface, which is the case this exists to refuse: a
		// missing host is not a local one.
		{":4321", false},
		{"0.0.0.0:4321", false},
		{"[::]:4321", false},
		{"192.168.1.10:4321", false},
		{"telecraft.example:4321", false},
		{"", false},
		{"127.0.0.1", false},
	} {
		if got := loopbackAddr(tc.endpoint); got != tc.want {
			t.Errorf("loopbackAddr(%q) = %v, want %v", tc.endpoint, got, tc.want)
		}
	}
}

func tree(t *testing.T) ownership.Tree {
	t.Helper()
	return ownership.Tree{
		Teams: map[ownership.TeamID]ownership.Team{
			"platform": {ID: "platform", Owners: []ownership.OwnerID{"platform-lead"}},
			"payments": {ID: "payments", Parent: "platform", Owners: []ownership.OwnerID{"pay-lead"}},
			"alpha":    {ID: "alpha", Owners: []ownership.OwnerID{"beta-owner", "alpha-owner"}},
		},
		Owners: map[ownership.OwnerID]ownership.Owner{
			"platform-lead": {},
			"pay-lead":      {},
			"alpha-owner":   {},
			"beta-owner":    {},
		},
	}
}

// The tree is a map, so the answer has to be sorted rather than whatever
// the runtime iterates first: a bootstrap that acted as a different owner
// each poll would be a different person each poll.
func TestRootOwnerIsDeterministic(t *testing.T) {
	first, err := rootOwner(tree(t))
	if err != nil {
		t.Fatalf("rootOwner: %v", err)
	}
	if first != "alpha-owner" {
		t.Fatalf("rootOwner = %q, want the first owner of the first root team", first)
	}
	for range 50 {
		again, err := rootOwner(tree(t))
		if err != nil || again != first {
			t.Fatalf("rootOwner = %q, %v; want %q every time", again, err, first)
		}
	}
}

// A child team is never the answer, however early it sorts.
func TestRootOwnerSkipsChildren(t *testing.T) {
	only := ownership.Tree{
		Teams: map[ownership.TeamID]ownership.Team{
			"aaa":  {ID: "aaa", Parent: "root", Owners: []ownership.OwnerID{"child-owner"}},
			"root": {ID: "root", Owners: []ownership.OwnerID{"root-owner"}},
		},
		Owners: map[ownership.OwnerID]ownership.Owner{"child-owner": {}, "root-owner": {}},
	}
	got, err := rootOwner(only)
	if err != nil {
		t.Fatalf("rootOwner: %v", err)
	}
	if got != "root-owner" {
		t.Fatalf("rootOwner = %q, want root-owner", got)
	}
}

func TestRootOwnerRefusesATreeWithNoOwnedRoot(t *testing.T) {
	_, err := rootOwner(ownership.Tree{
		Teams:  map[ownership.TeamID]ownership.Team{"root": {ID: "root"}},
		Owners: map[ownership.OwnerID]ownership.Owner{},
	})
	if err == nil || !strings.Contains(err.Error(), ownership.TeamsFile) {
		t.Fatalf("err = %v, want one naming %s", err, ownership.TeamsFile)
	}
}

// Off loopback the absence stays fatal, and the message says what to do.
func TestBootstrapRefusedOffLoopback(t *testing.T) {
	s := &Server{cfg: Config{HTTPEndpoint: "0.0.0.0:4321", Root: "/estate"}, logf: func(string, ...any) {}}
	_, err := s.bootstrapUsers(tree(t))
	if err == nil {
		t.Fatal("bootstrapUsers off loopback = nil, want a refusal")
	}
	for _, want := range []string{"users.yaml", "another host"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// The secret is drawn once. refreshAuth runs on every poll, and a password
// that changed every thirty seconds would be no password at all.
func TestBootstrapSecretIsDrawnOnce(t *testing.T) {
	var said int
	s := &Server{
		cfg:  Config{HTTPEndpoint: "127.0.0.1:4321", Root: "/estate"},
		logf: func(format string, args ...any) { said++ },
	}
	if _, err := s.bootstrapUsers(tree(t)); err != nil {
		t.Fatalf("bootstrapUsers: %v", err)
	}
	first := s.bootstrapSecret
	if len(first) != 32 {
		t.Fatalf("secret is %d hex characters, want 32", len(first))
	}
	lines := said
	for range 5 {
		if _, err := s.bootstrapUsers(tree(t)); err != nil {
			t.Fatalf("bootstrapUsers: %v", err)
		}
	}
	if s.bootstrapSecret != first {
		t.Fatal("the secret changed between polls")
	}
	if said != lines {
		t.Fatalf("the credential was printed again on a later poll (%d lines, then %d)", lines, said)
	}
}

// Two servers do not share a secret.
func TestBootstrapSecretIsPerProcess(t *testing.T) {
	mint := func() string {
		s := &Server{cfg: Config{HTTPEndpoint: "127.0.0.1:4321"}, logf: func(string, ...any) {}}
		if _, err := s.bootstrapUsers(tree(t)); err != nil {
			t.Fatalf("bootstrapUsers: %v", err)
		}
		return s.bootstrapSecret
	}
	if mint() == mint() {
		t.Fatal("two servers drew the same bootstrap secret")
	}
}
