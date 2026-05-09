package stack

import "testing"

func TestProbeTargetsUsesExplicitProbes(t *testing.T) {
	cfg := FailoverConfig{
		ProbeIP:   "old",
		ProbePort: 123,
		Probes: []ProbeTarget{
			{Address: "one", Port: 443},
			{Address: "two"},
			{},
		},
	}
	got := cfg.ProbeTargets()
	if len(got) != 2 {
		t.Fatalf("expected two probes: %+v", got)
	}
	if got[0] != (ProbeTarget{Address: "one", Port: 443}) {
		t.Fatalf("unexpected first probe: %+v", got[0])
	}
	if got[1] != (ProbeTarget{Address: "two", Port: 443}) {
		t.Fatalf("unexpected second probe: %+v", got[1])
	}
}

func TestProbeTargetsFallsBackToLegacyProbe(t *testing.T) {
	got := (FailoverConfig{ProbeIP: "legacy", ProbePort: 8443}).ProbeTargets()
	if len(got) != 1 || got[0] != (ProbeTarget{Address: "legacy", Port: 8443}) {
		t.Fatalf("unexpected probes: %+v", got)
	}
}
