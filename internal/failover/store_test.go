package failover

import (
	"path/filepath"
	"testing"
)

func TestStateStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	want := State{
		Candidate:      Mode{OutboundTag: "proxy", Interface: "tun1"},
		Count:          3,
		LastChangeUnix: 123,
	}
	if err := SaveState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("state mismatch: got %+v want %+v", got, want)
	}
}

func TestLoadStateMissingFile(t *testing.T) {
	got, err := LoadState(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got != (State{}) {
		t.Fatalf("expected empty state: %+v", got)
	}
}
