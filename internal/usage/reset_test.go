package usage

import (
	"testing"
	"time"
)

func TestApplyPeriodResetsSeedsDeadlines(t *testing.T) {
	loc, _ := time.LoadLocation("Local")
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, loc) // Sun
	st := ensureUser(UserState{})
	st.Daily["amir"] = Pair{Uplink: 100, Downlink: 200}
	users := []PeriodUser{{Email: "amir", DailyResetHHMM: "00:00"}}

	changed, next := ApplyPeriodResets(st, users, now)
	if !changed {
		t.Fatalf("first call should seed deadlines and report changed=true")
	}
	if next.Daily["amir"].Uplink != 100 {
		t.Errorf("counter should not be reset on first observation, got %+v", next.Daily["amir"])
	}
	meta := next.Periods["amir"]
	if meta.DailyResetAt == 0 || meta.WeeklyResetAt == 0 || meta.MonthlyResetAt == 0 {
		t.Errorf("all deadlines should be seeded, got %+v", meta)
	}
}

func TestApplyPeriodResetsRollsOverDaily(t *testing.T) {
	loc, _ := time.LoadLocation("Local")
	// 23:30, daily reset at 00:00 → after midnight the daily counter
	// should be zeroed and the next deadline pushed to tomorrow.
	now := time.Date(2026, 5, 17, 23, 30, 0, 0, loc)
	st := ensureUser(UserState{})
	st.Daily["amir"] = Pair{Uplink: 50, Downlink: 60}
	st.Periods["amir"] = PeriodMeta{
		DailyResetAt:   time.Date(2026, 5, 17, 23, 0, 0, 0, loc).Unix(), // already past
		WeeklyResetAt:  now.AddDate(0, 0, 7).Unix(),
		MonthlyResetAt: now.AddDate(0, 1, 0).Unix(),
	}
	users := []PeriodUser{{Email: "amir", DailyResetHHMM: "00:00"}}

	changed, next := ApplyPeriodResets(st, users, now)
	if !changed {
		t.Fatalf("expected changed=true after deadline elapsed")
	}
	if next.Daily["amir"] != (Pair{}) {
		t.Errorf("daily counter should be zeroed, got %+v", next.Daily["amir"])
	}
	if next.Periods["amir"].DailyResetAt <= now.Unix() {
		t.Errorf("daily deadline should be advanced past now, got %d (now=%d)", next.Periods["amir"].DailyResetAt, now.Unix())
	}
}

func TestNextWeeklyResetIsAlwaysNextMondayMidnight(t *testing.T) {
	loc, _ := time.LoadLocation("Local")
	cases := []struct {
		now time.Time
	}{
		{time.Date(2026, 5, 17, 10, 0, 0, 0, loc)},  // Sun → Mon (next day)
		{time.Date(2026, 5, 18, 0, 0, 1, 0, loc)},   // Mon 00:00:01 → next Mon (a week)
		{time.Date(2026, 5, 22, 23, 59, 0, 0, loc)}, // Fri
	}
	for _, c := range cases {
		got := nextWeeklyReset(c.now)
		if got.Weekday() != time.Monday {
			t.Errorf("nextWeeklyReset(%v) weekday = %v, want Monday", c.now, got.Weekday())
		}
		if got.Hour() != 0 || got.Minute() != 0 {
			t.Errorf("nextWeeklyReset(%v) time = %02d:%02d, want 00:00", c.now, got.Hour(), got.Minute())
		}
		if !got.After(c.now) {
			t.Errorf("nextWeeklyReset(%v) = %v, must be strictly after", c.now, got)
		}
	}
}
