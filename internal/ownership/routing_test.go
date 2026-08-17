package ownership

import (
	"strings"
	"testing"
)

// Acceptance: a finding routes to the owner of the object it is about —
// the broken PII processor pages the security team, the unmet Service floor
// pages the Service's owner (ADR-0016 §4).
func TestFindingRoutesToOwnerOfSubjectObject(t *testing.T) {
	est := loadFixture(t)

	cases := map[string]struct {
		subject   Subject
		wantOwner OwnerID
		wantTeam  TeamID
	}{
		"component finding pages the component's owner": {
			subject:   Subject{Kind: KindComponent, ID: "infosec/pii-redaction"},
			wantOwner: "pii-guardians",
			wantTeam:  "infosec",
		},
		"exporter finding pages the data-flow team, not the rendered file's owner": {
			subject:   Subject{Kind: KindComponent, ID: "data-flow/gateway-exporter"},
			wantOwner: "gateway-owners",
			wantTeam:  "data-flow",
		},
		"service floor finding pages the service's owner": {
			subject:   Subject{Kind: KindService, ID: "checkout"},
			wantOwner: "checkout-team",
			wantTeam:  "product",
		},
		"tier finding pages the tier's owner": {
			subject:   Subject{Kind: KindTier, ID: "edge"},
			wantOwner: "platform-observability",
			wantTeam:  "platform",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			owner, err := est.OwnerOf(tc.subject)
			if err != nil {
				t.Fatal(err)
			}
			if owner.ID != tc.wantOwner || owner.Team != tc.wantTeam {
				t.Errorf("OwnerOf(%v) = %+v, want owner %q of team %q", tc.subject, owner, tc.wantOwner, tc.wantTeam)
			}
		})
	}
}

// Acceptance: a collector inherits ownership from the Tier it matched into
// (ADR-0016 §5) — and the split Tier is how a subset gets a different owner.
func TestCollectorFindingInheritsTierOwner(t *testing.T) {
	est := loadFixture(t)

	gateway, err := est.OwnerOf(Subject{Kind: KindCollector, ID: "host-17", Tier: "gateway"})
	if err != nil {
		t.Fatal(err)
	}
	if gateway.ID != "gateway-owners" {
		t.Errorf("collector in tier gateway routed to %q, want gateway-owners", gateway.ID)
	}

	// The same collector population, split into gateway-pci by selector,
	// routes to the split Tier's owner — no per-collector override exists.
	pci, err := est.OwnerOf(Subject{Kind: KindCollector, ID: "host-17", Tier: "gateway-pci"})
	if err != nil {
		t.Fatal(err)
	}
	if pci.ID != "pii-guardians" {
		t.Errorf("collector in split tier gateway-pci routed to %q, want pii-guardians", pci.ID)
	}
}

func TestCollectorSubjectWithoutTierIsAnError(t *testing.T) {
	est := loadFixture(t)
	_, err := est.OwnerOf(Subject{Kind: KindCollector, ID: "host-17"})
	if err == nil || !strings.Contains(err.Error(), "Tier") {
		t.Fatalf("expected a no-tier error, got %v", err)
	}
}

func TestCollectorInUnknownTierIsAnError(t *testing.T) {
	est := loadFixture(t)
	_, err := est.OwnerOf(Subject{Kind: KindCollector, ID: "host-17", Tier: "no-such-tier"})
	if err == nil || !strings.Contains(err.Error(), "no-such-tier") {
		t.Fatalf("expected an unknown-tier error, got %v", err)
	}
}

func TestUnknownSubjectIsAnError(t *testing.T) {
	est := loadFixture(t)
	_, err := est.OwnerOf(Subject{Kind: KindService, ID: "no-such-service"})
	if err == nil || !strings.Contains(err.Error(), "routes nowhere") {
		t.Fatalf("expected a routes-nowhere error, got %v", err)
	}
}

func TestNonCollectorSubjectWithTierIsAnError(t *testing.T) {
	est := loadFixture(t)
	_, err := est.OwnerOf(Subject{Kind: KindService, ID: "checkout", Tier: "gateway"})
	if err == nil || !strings.Contains(err.Error(), "collector") {
		t.Fatalf("expected an error rejecting a tier on a non-collector subject, got %v", err)
	}
}
