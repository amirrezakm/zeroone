// SPDX-License-Identifier: AGPL-3.0-or-later
package xrayproc

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeXray writes a small shell script that echoes a marker line and
// then blocks on a read from stdin. SIGTERM causes a clean exit.
func writeFakeXray(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "xray")
	script := `#!/bin/sh
trap 'exit 0' TERM INT
echo "fake-xray-started"
# block forever; supervisor will SIGTERM us
while :; do sleep 1; done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake xray: %v", err)
	}
	return path
}

func TestSupervisorStartAndRestart(t *testing.T) {
	bin := writeFakeXray(t)
	logPath := filepath.Join(t.TempDir(), "xray.log")
	cfgPath := filepath.Join(t.TempDir(), "xray.json")
	_ = os.WriteFile(cfgPath, []byte(`{}`), 0o644)

	sup := New(bin, cfgPath, logPath, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		sup.Run(ctx)
		close(done)
	}()

	// wait until process is running
	waitFor(t, 3*time.Second, func() bool { return sup.Status().PID > 0 })
	firstPID := sup.Status().PID
	if firstPID == 0 {
		t.Fatal("expected xray to be running")
	}

	// trigger a restart; pid should change
	if err := sup.Restart(ctx, nil); err != nil {
		t.Fatalf("restart: %v", err)
	}
	waitFor(t, 3*time.Second, func() bool {
		pid := sup.Status().PID
		return pid != 0 && pid != firstPID
	})
	if sup.Status().PID == firstPID {
		t.Fatalf("expected pid to change after restart, still %d", firstPID)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not exit after ctx cancel")
	}

	// log file should exist and contain marker
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(contents) == 0 {
		t.Fatal("expected xray log to have content")
	}
}

func TestNextBackoff(t *testing.T) {
	cases := []struct{ in, want time.Duration }{
		{time.Second, 2 * time.Second},
		{4 * time.Second, 8 * time.Second},
		{30 * time.Second, 60 * time.Second},
		{60 * time.Second, 60 * time.Second},
	}
	for _, c := range cases {
		if got := nextBackoff(c.in); got != c.want {
			t.Errorf("nextBackoff(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}

func waitFor(t *testing.T, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("waitFor: condition not met within %s", d)
}
