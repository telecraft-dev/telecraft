package main

import (
	"testing"

	"github.com/open-telemetry/opamp-go/protobufs"
)

// Reading a collector's delivery path off the serving wire (REQ-041).
//
// Every assertion below is about the answer coming from what the collector
// declared, and about it dying with the connection that declared it. A
// devenv that inferred the path from a list of collector names would agree
// with these tests today and be wrong the first time somebody added one.

var (
	acceptsRemoteConfig = uint64(protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig |
		protobufs.AgentCapabilities_AgentCapabilities_ReportsRemoteConfig |
		protobufs.AgentCapabilities_AgentCapabilities_ReportsEffectiveConfig)

	reportsOnly = uint64(protobufs.AgentCapabilities_AgentCapabilities_ReportsEffectiveConfig |
		protobufs.AgentCapabilities_AgentCapabilities_ReportsHealth)
)

func TestACollectorThatAcceptsRemoteConfigIsServed(t *testing.T) {
	identity := map[string]string{"telecraft.tier": "gateway", "service.instance.id": "gateway-1"}
	d := &deliveryPaths{}

	d.Report("conn-1", identity, &protobufs.AgentToServer{Capabilities: acceptsRemoteConfig})

	if got := d.Path(identity); got != "served" {
		t.Errorf("delivery %q: this collector will apply what the server sends it, which is the served path", got)
	}
}

func TestACollectorThatTakesNothingTheServerSendsIsGitDelivered(t *testing.T) {
	identity := map[string]string{"telecraft.tier": "appliance", "service.instance.id": "appliance-1"}
	d := &deliveryPaths{}

	// The collector's own opamp extension: it reports and has no
	// remote-config side at all, so the platform sees what is running and
	// something else decides it.
	d.Report("conn-1", identity, &protobufs.AgentToServer{Capabilities: reportsOnly})

	if got := d.Path(identity); got != "git" {
		t.Errorf("delivery %q: nothing the platform sends this collector would be applied, so it is not served", got)
	}
}

func TestADeclarationWithoutIdentityWaitsForOne(t *testing.T) {
	identity := map[string]string{"telecraft.tier": "appliance"}
	d := &deliveryPaths{}

	// The first message on a connection can arrive without the agent
	// description; the server asks for full state and the next one carries
	// it. Filing the capabilities under no collector would lose them.
	d.Report("conn-1", nil, &protobufs.AgentToServer{Capabilities: reportsOnly})
	d.Report("conn-1", identity, &protobufs.AgentToServer{Capabilities: reportsOnly})

	if got := d.Path(identity); got != "git" {
		t.Errorf("delivery %q: the declaration made before the identity arrived was lost", got)
	}
}

func TestTheDeclarationDiesWithTheConnection(t *testing.T) {
	identity := map[string]string{"telecraft.tier": "appliance"}
	d := &deliveryPaths{}
	d.Report("conn-1", identity, &protobufs.AgentToServer{Capabilities: reportsOnly})

	d.Closed("conn-1")

	// Everything a tap holds is derivable from live connections and dies
	// with them (ADR-0032). Nothing asks about a collector that has gone,
	// and holding its answer would be a cache with no way to expire.
	if got := d.Path(identity); got != "served" {
		t.Errorf("delivery %q: a closed connection's declaration outlived it", got)
	}
}

func TestAReconnectingCollectorIsStillServedOverItsOtherConnection(t *testing.T) {
	identity := map[string]string{"telecraft.tier": "gateway", "service.instance.id": "gateway-1"}
	d := &deliveryPaths{}

	// A reconnect leaves two live connections for one collector for a
	// moment. The platform is delivering to it over one of them, so the
	// half-open one must not flip the whole Tier's delivery split.
	d.Report("conn-1", identity, &protobufs.AgentToServer{Capabilities: acceptsRemoteConfig})
	d.Report("conn-2", identity, &protobufs.AgentToServer{})

	if got := d.Path(identity); got != "served" {
		t.Errorf("delivery %q: a second connection made a served collector read as git-delivered", got)
	}
}

func TestOneCollectorsPathSaysNothingAboutAnother(t *testing.T) {
	served := map[string]string{"telecraft.tier": "gateway", "service.instance.id": "gateway-1"}
	git := map[string]string{"telecraft.tier": "appliance", "service.instance.id": "appliance-1"}
	d := &deliveryPaths{}

	d.Report("conn-1", served, &protobufs.AgentToServer{Capabilities: acceptsRemoteConfig})
	d.Report("conn-2", git, &protobufs.AgentToServer{Capabilities: reportsOnly})

	if d.Path(served) != "served" || d.Path(git) != "git" {
		t.Errorf("served read as %q and git-delivered as %q: the two paths ran together", d.Path(served), d.Path(git))
	}
}
