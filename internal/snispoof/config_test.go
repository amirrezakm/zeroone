package snispoof

import (
	"strings"
	"testing"

	"github.com/amirrezakm/zeroone/internal/stack"
)

func argsString(a []string) string { return strings.Join(a, " ") }

func TestByeDPIArgsFakeMethod(t *testing.T) {
	c := stack.SNISpoofConfig{
		Enabled:    true,
		Listen:     "127.0.0.1:8087",
		Method:     "fake",
		FakeDomain: "www.hcaptcha.com",
		FakeTTL:    8,
	}
	args, err := ByeDPIArgs(c)
	if err != nil {
		t.Fatal(err)
	}
	got := argsString(args)
	for _, want := range []string{"-i 127.0.0.1", "-p 8087", "-f 1+s", "-t 8", "-n www.hcaptcha.com", "-K t"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fake args missing %q in %q", want, got)
		}
	}
}

func TestByeDPIArgsDefaultsWhenEmpty(t *testing.T) {
	c := stack.SNISpoofConfig{Enabled: true}
	args, err := ByeDPIArgs(c)
	if err != nil {
		t.Fatal(err)
	}
	got := argsString(args)
	// method defaults to "fake", fake domain + ttl defaulted.
	if !strings.Contains(got, "-n "+stack.DefaultSNISpoofFakeDomain) {
		t.Fatalf("expected default fake domain in %q", got)
	}
	if !strings.Contains(got, "-p 8087") {
		t.Fatalf("expected default listen port in %q", got)
	}
}

func TestByeDPIArgsSplitAndDisorder(t *testing.T) {
	for _, m := range []string{"split", "disorder"} {
		c := stack.SNISpoofConfig{Enabled: true, Method: m}
		args, err := ByeDPIArgs(c)
		if err != nil {
			t.Fatalf("%s: %v", m, err)
		}
		got := argsString(args)
		flag := "-s 1+s"
		if m == "disorder" {
			flag = "-d 1+s"
		}
		if !strings.Contains(got, flag) {
			t.Fatalf("%s args missing %q in %q", m, flag, got)
		}
		if strings.Contains(got, "-n ") {
			t.Fatalf("%s should not inject a fake SNI: %q", m, got)
		}
	}
}

func TestByeDPIArgsStrategyOverride(t *testing.T) {
	c := stack.SNISpoofConfig{
		Enabled:   true,
		Listen:    "127.0.0.1:9000",
		Strategy:  "-s 4 -d 2",
		ExtraArgs: []string{"-Y"},
	}
	args, err := ByeDPIArgs(c)
	if err != nil {
		t.Fatal(err)
	}
	got := argsString(args)
	if !strings.Contains(got, "-s 4 -d 2") {
		t.Fatalf("strategy override missing in %q", got)
	}
	if !strings.HasSuffix(got, "-Y") {
		t.Fatalf("extra args should be appended last: %q", got)
	}
	// when Strategy is set the method preset must not leak in.
	if strings.Contains(got, "-K t") {
		t.Fatalf("method preset leaked despite strategy override: %q", got)
	}
}

func TestTun2socksArgs(t *testing.T) {
	c := stack.SNISpoofConfig{
		Enabled: true,
		Listen:  "127.0.0.1:8087",
		TunName: "znspoof0",
		MTU:     1400,
	}
	args := Tun2socksArgs(c, "info")
	got := argsString(args)
	for _, want := range []string{"-device tun://znspoof0", "-proxy socks5://127.0.0.1:8087", "-loglevel info", "-mtu 1400"} {
		if !strings.Contains(got, want) {
			t.Fatalf("tun2socks args missing %q in %q", want, got)
		}
	}
}

func TestParseHostPort(t *testing.T) {
	h, p, err := parseHostPort("www.google.com:443")
	if err != nil || h != "www.google.com" || p != 443 {
		t.Fatalf("got %q %d %v", h, p, err)
	}
	h, p, err = parseHostPort("example.com")
	if err != nil || h != "example.com" || p != 443 {
		t.Fatalf("bare host: got %q %d %v", h, p, err)
	}
}
