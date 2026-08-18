package inventory

import (
	"testing"
	"time"
)

// The source ranking (ADR-0035 §2): derived > declared > absent.
func TestResolveFloorRanking(t *testing.T) {
	asOf := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		derived  Count
		declared int
		want     Floor
	}{
		{
			name:     "a known derived count wins over a declaration",
			derived:  Count{Known: true, AsOf: asOf, Instances: 12},
			declared: 40,
			want:     Floor{Source: FloorDerived, Min: 12, AsOf: asOf},
		},
		{
			name:     "a derived zero is still derived — the substrate honestly expecting nothing",
			derived:  Count{Known: true, AsOf: asOf, Instances: 0},
			declared: 40,
			want:     Floor{Source: FloorDerived, Min: 0, AsOf: asOf},
		},
		{
			name:     "a declaration stands in when the derived count is unknown",
			derived:  Count{Known: false, Cause: "unreachable", AsOf: asOf},
			declared: 12,
			want:     Floor{Source: FloorDeclared, Min: 12},
		},
		{
			name: "no provider and no declaration is no floor — no teeth, and nobody guesses",
			want: Floor{},
		},
		{
			name:     "a zero declaration is no declaration",
			declared: 0,
			want:     Floor{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveFloor(tc.derived, tc.declared); got != tc.want {
				t.Fatalf("ResolveFloor = %+v, want %+v", got, tc.want)
			}
		})
	}
}
