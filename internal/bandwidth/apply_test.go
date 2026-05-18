package bandwidth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/system"
)

type fakeRunner struct {
	calls []string
	fail  string
}

func (r *fakeRunner) Run(ctx context.Context, name string, args ...string) (system.Result, error) {
	call := name + " " + strings.Join(args, " ")
	r.calls = append(r.calls, call)
	if r.fail != "" && strings.Contains(call, r.fail) {
		return system.Result{Stderr: "boom"}, errors.New("failed")
	}
	return system.Result{}, nil
}

func TestApplyRunsLimitCommands(t *testing.T) {
	runner := &fakeRunner{}
	cfg := stack.Config{
		Server: stack.ServerConfig{BandwidthDevice: "eth0"},
		Xray: stack.XrayConfig{Users: []stack.User{
			{Email: "amir", Enabled: true, BandwidthPort: 21000, DownloadMbps: 10, UploadMbps: 2},
		}},
	}
	result, err := (Manager{Runner: runner}).Apply(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || len(result.Limits) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	joined := strings.Join(runner.calls, "\n")
	if !strings.Contains(joined, "nft -f") || !strings.Contains(joined, "dst_port 21000") {
		t.Fatalf("missing expected calls:\n%s", joined)
	}
}

func TestApplyIgnoresMissingExistingRulesWhenClearing(t *testing.T) {
	runner := &fakeRunner{fail: "delete table"}
	cfg := stack.Config{Server: stack.ServerConfig{BandwidthDevice: "eth0"}}
	result, err := (Manager{Runner: runner}).Apply(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied {
		t.Fatalf("expected applied result: %+v", result)
	}
}
