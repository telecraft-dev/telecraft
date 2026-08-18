package auth

import (
	"strings"
	"testing"
	"time"
)

func testSessions(t *testing.T) Sessions {
	t.Helper()
	s, err := NewSessions([]byte(strings.Repeat("k", 32)), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSessionRoundTrip(t *testing.T) {
	s := testSessions(t)
	id := Identity{Subject: "sub-1", Name: "Jo Author", Email: "jo@example.com"}
	token, err := s.Issue(id)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Fatalf("Verify returned %+v, want %+v", got, id)
	}
}

func TestSessionTamperingFails(t *testing.T) {
	s := testSessions(t)
	token, err := s.Issue(Identity{Subject: "sub-1", Name: "Jo", Email: "jo@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		"",
		"v1.garbage",
		token + "x",
		"v1." + strings.SplitN(token, ".", 3)[1] + ".forged-signature",
	} {
		if _, err := s.Verify(bad); err == nil {
			t.Fatalf("Verify accepted %q", bad)
		}
	}
	// A token signed under another key never verifies.
	other, err := NewSessions([]byte(strings.Repeat("x", 32)), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Verify(token); err == nil {
		t.Fatal("a token crossed signing keys")
	}
}

func TestSessionExpires(t *testing.T) {
	s := testSessions(t)
	token, err := s.Issue(Identity{Subject: "sub-1", Name: "Jo", Email: "jo@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := s.Verify(token); err == nil {
		t.Fatal("Verify accepted an expired token")
	}
}

func TestSessionKeyFloor(t *testing.T) {
	if _, err := NewSessions([]byte("short"), 0); err == nil {
		t.Fatal("NewSessions accepted a key below the floor")
	}
	// An empty key draws a random one — the standalone shape.
	s, err := NewSessions(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Issue(Identity{Subject: "s", Name: "n", Email: "e@example.com"}); err != nil {
		t.Fatal(err)
	}
}
