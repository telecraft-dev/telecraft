package main

import (
	"sync"

	"github.com/open-telemetry/opamp-go/protobufs"

	seam "github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/serving"
)

// deliveryPaths reads how each collector came by its configuration — the
// served path or the git-delivered one (REQ-041).
//
// The signal is a declaration and not an absence. Every OpAMP message
// carries the agent's capability bitmask, and AcceptsRemoteConfig is the
// collector saying whether it will take a configuration from the server it
// is talking to. A Supervisor sets it, because serving is the point
// (ADR-0010). The collector's own opamp extension cannot set it: that
// extension reports effective config and health and has no remote-config
// side at all. So a collector reporting to this server without the
// capability is one the platform sees and does not deliver to, which is the
// Foreign path exactly.
//
// Reading the absence of a remote-config *status* report would be the
// cheaper answer and the wrong one: a served collector has not reported one
// either, for the first moments of its connection.
//
// This is a devenv reading rather than a seam one. The EstateProvider seam
// carries what a collector reports about its own running state (ADR-0008);
// where that state came from is a different question, and answering it in
// core would be a development environment deciding product semantics ahead
// of the product (ADR-0052 §3).
//
// Like every other tap, what it holds is a cache of live connections and
// dies with them (ADR-0032).
type deliveryPaths struct {
	mu   sync.Mutex
	held map[any]connectionPath
}

// connectionPath is one live connection's answer: whose collector it is,
// and whether that collector accepts what this server sends.
type connectionPath struct {
	fingerprint string
	accepts     bool
}

func (d *deliveryPaths) Report(conn any, identity map[string]string, msg *protobufs.AgentToServer) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.held == nil {
		d.held = map[any]connectionPath{}
	}
	held := d.held[conn]
	if len(identity) > 0 {
		held.fingerprint = seam.Fingerprint(identity)
	}
	// Carried on every message rather than only the first: opamp-go
	// re-sends the capability bitmask with each one, so this is the
	// collector's current declaration and not a memory of its handshake.
	held.accepts = msg.GetCapabilities()&uint64(protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig) != 0
	d.held[conn] = held
}

func (d *deliveryPaths) Closed(conn any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.held, conn)
}

// Path answers the delivery path for the collector reporting these
// attributes.
//
// A reconnect can briefly leave two live connections for one collector, so
// any connection of that collector still accepting remote config makes it
// served: the platform is delivering to it over one of them.
//
// A collector this wire has shown nothing about reads as served, because
// REQ-041 has two values and no third for not having looked. It cannot
// arise from the composer, which only asks about collectors the same wire
// reported.
func (d *deliveryPaths) Path(identity map[string]string) string {
	key := seam.Fingerprint(identity)
	d.mu.Lock()
	defer d.mu.Unlock()
	seen := false
	for _, held := range d.held {
		if held.fingerprint != key {
			continue
		}
		if held.accepts {
			return "served"
		}
		seen = true
	}
	if seen {
		return "git"
	}
	return "served"
}

var (
	_ serving.Tap    = (*deliveryPaths)(nil)
	_ deliveryReader = (*deliveryPaths)(nil)
)
