package analytics

import (
	"path/filepath"
	"testing"
	"time"
)

func TestIngestParsesAcceptedLines(t *testing.T) {
	a := New(filepath.Join(t.TempDir(), "dest.json"))
	a.retention = 48 * time.Hour
	sample := `2026-05-10T20:47:38+03:30 server xray[1]: from 81.29.255.34:60217 accepted tcp:178.22.122.200:443 [vless-public -> block] email: parastoo
2026-05-10T20:47:39+03:30 server xray[1]: from 81.29.255.34:60219 accepted tcp:audio-ssl.itunes.apple.com:443 [vless-public >> proxy] email: parastoo
2026-05-10T20:47:40+03:30 server xray[1]: from 81.29.255.34:60220 accepted tcp:audio-ssl.itunes.apple.com:443 [vless-public >> proxy] email: parastoo
2026-05-10T20:47:41+03:30 server xray[1]: from DNS accepted udp:1.1.1.1:53 [vless-public >> proxy] email: parastoo
-- cursor: s=abc;i=def
`
	a.ingest(sample)
	res := a.Top(10)
	if res.Total != 4 {
		t.Fatalf("want total 4 got %d", res.Total)
	}
	if len(res.Items) < 3 {
		t.Fatalf("want >=3 items got %d", len(res.Items))
	}
	if res.Items[0].Destination != "audio-ssl.itunes.apple.com" || res.Items[0].Requests != 2 {
		t.Fatalf("apple should be top with 2: %+v", res.Items[0])
	}
	if a.state.LastCursor != "s=abc;i=def" {
		t.Fatalf("cursor not captured: %q", a.state.LastCursor)
	}
}

func TestPruneDropsOldBuckets(t *testing.T) {
	a := New(filepath.Join(t.TempDir(), "dest.json"))
	a.retention = 1 * time.Hour
	now := time.Now().Unix()
	a.state.Buckets = []HourBucket{
		{Hour: now - 7200, Destinations: map[string]int64{"old.example": 99}, Total: 99},
		{Hour: now - (now % 3600), Destinations: map[string]int64{"new.example": 1}, Total: 1},
	}
	a.prune(now - 3600)
	if len(a.state.Buckets) != 1 || a.state.Buckets[0].Destinations["new.example"] != 1 {
		t.Fatalf("prune wrong: %+v", a.state.Buckets)
	}
}

func TestStripPort(t *testing.T) {
	for in, want := range map[string]string{
		"google.com:443":     "google.com",
		"1.2.3.4:443":        "1.2.3.4",
		"[::1]:443":          "::1",
		"[2606:4700::]:80":   "2606:4700::",
		"":                   "",
		"hostonly":           "hostonly",
	} {
		if got := stripPort(in); got != want {
			t.Errorf("stripPort(%q) = %q want %q", in, got, want)
		}
	}
}
