package usage

import (
	"testing"
	"time"
)

func TestSyncUsersPreservesTotalsAcrossRawReset(t *testing.T) {
	st := SyncUsers(UserState{}, map[string]Pair{"amir": {Uplink: 100, Downlink: 900}}, time.Unix(10, 0))
	if st.Totals["amir"].Uplink != 100 || st.Totals["amir"].Downlink != 900 {
		t.Fatalf("unexpected totals: %+v", st.Totals["amir"])
	}
	st = SyncUsers(st, map[string]Pair{"amir": {Uplink: 10, Downlink: 30}}, time.Unix(20, 0))
	if st.Totals["amir"].Uplink != 110 || st.Totals["amir"].Downlink != 930 {
		t.Fatalf("raw reset not handled: %+v", st.Totals["amir"])
	}
}

func TestResetUsersSetsBaselineToCurrentRaw(t *testing.T) {
	st := SyncUsers(UserState{}, map[string]Pair{"amir": {Uplink: 100, Downlink: 900}}, time.Unix(10, 0))
	st = ResetUsers(st, map[string]Pair{"amir": {Uplink: 150, Downlink: 1000}}, []string{"amir", "vigen"}, time.Unix(20, 0))
	if st.Totals["amir"] != (Pair{}) || st.LastRaw["amir"].Downlink != 1000 {
		t.Fatalf("reset mismatch: %+v", st)
	}
	st = SyncUsers(st, map[string]Pair{"amir": {Uplink: 170, Downlink: 1010}}, time.Unix(30, 0))
	if st.Totals["amir"].Uplink != 20 || st.Totals["amir"].Downlink != 10 {
		t.Fatalf("post-reset delta mismatch: %+v", st.Totals["amir"])
	}
}

func TestSplitStat(t *testing.T) {
	parts := splitStat("user>>>amir>>>traffic>>>downlink")
	if len(parts) != 4 || parts[1] != "amir" || parts[3] != "downlink" {
		t.Fatalf("bad split: %#v", parts)
	}
}
