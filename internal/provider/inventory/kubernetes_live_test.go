package inventory

// Live verification against a real Kubernetes API server (issue #24).
//
// These tests are gated on TELECRAFT_INVENTORY_LIVE_ENDPOINT and skip
// loudly when it is unset, so a plain `go test ./...` stays green on a
// machine with no cluster. The lowest-friction gate is a local proxy:
//
//	kubectl proxy &
//	TELECRAFT_INVENTORY_LIVE_ENDPOINT=http://127.0.0.1:8001 go test ./internal/provider/inventory/ -run Live -v
//
// or point the gate at an API server directly and pass a bearer token in
// TELECRAFT_INVENTORY_LIVE_TOKEN (the client trusts the system CA pool;
// a self-signed cluster is what the proxy form is for). Read access to
// nodes is all the suite needs.

import (
	"context"
	"os"
	"testing"
)

const (
	liveGate  = "TELECRAFT_INVENTORY_LIVE_ENDPOINT"
	liveToken = "TELECRAFT_INVENTORY_LIVE_TOKEN"
)

func liveProvider(t *testing.T) *Kubernetes {
	t.Helper()
	endpoint := os.Getenv(liveGate)
	if endpoint == "" {
		t.Skipf("set %s to run against a live Kubernetes API server (e.g. kubectl proxy at http://127.0.0.1:8001)", liveGate)
	}
	p, err := NewKubernetes(KubernetesConfig{Endpoint: endpoint, Token: os.Getenv(liveToken)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Every live node carries kubernetes.io/os, so the selector must derive a
// positive count — the real cluster answering the seam's one question.
func TestLiveNodesDeriveACount(t *testing.T) {
	c := liveProvider(t).Expected(context.Background(), map[string]string{"kubernetes.io/os": "linux"})
	switch {
	case !c.Known:
		t.Fatalf("the live cluster could not be counted: %s", c.Cause)
	case c.Instances < 1:
		t.Fatalf("Instances = %d — a live cluster has at least one linux node", c.Instances)
	case c.AsOf.IsZero():
		t.Fatal("the live count carries no as_of")
	}
}

// A selector matching no node is a count of zero — a real reading from
// the substrate, never a blind spot and never an invention (ADR-0035 §2).
func TestLiveAbsentSelectorIsZero(t *testing.T) {
	c := liveProvider(t).Expected(context.Background(), map[string]string{"telecraft.live-test/absent": "matches-nothing"})
	if !c.Known || c.Instances != 0 {
		t.Fatalf("absent selector: %+v, want a Known zero", c)
	}
}

// The empty selector stays unanswerable against a real cluster too.
func TestLiveEmptySelectorIsUnknown(t *testing.T) {
	if c := liveProvider(t).Expected(context.Background(), nil); c.Known {
		t.Fatalf("the empty selector became a count: %+v", c)
	}
}
