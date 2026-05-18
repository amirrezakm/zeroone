package relay

import (
	"strings"
	"testing"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

func TestDeploymentIDsParsesURLs(t *testing.T) {
	c := stack.RelayConfig{
		ScriptURL:     "https://script.google.com/macros/s/AKfycbwabc123/exec",
		DeploymentIDs: []string{"AKfycbDIRECT", "https://script.google.com/macros/s/AKfycbURL/exec", "AKfycbDIRECT"},
	}
	got := DeploymentIDs(c)
	want := []string{"AKfycbDIRECT", "AKfycbURL", "AKfycbwabc123"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("idx %d: want %q got %q", i, v, got[i])
		}
	}
}

func TestRenderConfigRequiresAuth(t *testing.T) {
	_, err := RenderConfig(stack.RelayConfig{DeploymentIDs: []string{"X"}})
	if err == nil {
		t.Fatalf("expected error for missing auth_key")
	}
}

func TestRenderConfigRequiresDeployment(t *testing.T) {
	_, err := RenderConfig(stack.RelayConfig{AuthKey: "secret"})
	if err == nil {
		t.Fatalf("expected error for missing deployment id")
	}
}

func TestRenderConfigEmitsRequiredFields(t *testing.T) {
	c := stack.RelayConfig{
		AuthKey:   "super-secret",
		Listen:    "127.0.0.1:8085",
		ScriptURL: "https://script.google.com/macros/s/AKfycbAAA/exec",
		Sites:     []stack.RelaySite{{Domain: "youtube.com", Enabled: true}},
	}
	raw, err := RenderConfig(c)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := string(raw)
	for _, want := range []string{
		`"mode": "apps_script"`,
		`"auth_key": "super-secret"`,
		`"listen_host": "127.0.0.1"`,
		`"listen_port": 8085`,
		`"script_id": "AKfycbAAA"`,
		`"google_ip"`,
		`"front_domain": "www.google.com"`,
		`"socks5_port"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered config missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderConfigMultipleDeploymentsAsArray(t *testing.T) {
	c := stack.RelayConfig{
		AuthKey:       "k",
		DeploymentIDs: []string{"AKfycbA", "AKfycbB"},
	}
	raw, err := RenderConfig(c)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(string(raw), `"script_id": [`) {
		t.Fatalf("expected script_id as array, got: %s", raw)
	}
}
