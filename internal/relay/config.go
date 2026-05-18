package relay

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

// DeploymentIDs returns the deployment IDs the relay should attempt, in
// order. Bare IDs and full Apps Script URLs are both accepted; URLs are
// reduced to the AKfyc... token between /macros/s/ and /exec.
func DeploymentIDs(c stack.RelayConfig) []string {
	seen := map[string]bool{}
	out := []string{}
	push := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		id := normaliseDeploymentID(raw)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, id := range c.DeploymentIDs {
		push(id)
	}
	if c.ScriptURL != "" {
		push(c.ScriptURL)
	}
	return out
}

func normaliseDeploymentID(raw string) string {
	if strings.Contains(raw, "/macros/s/") {
		u, err := url.Parse(raw)
		if err == nil {
			parts := strings.Split(u.Path, "/")
			for i, p := range parts {
				if p == "s" && i+1 < len(parts) {
					return parts[i+1]
				}
			}
		}
	}
	if i := strings.LastIndex(raw, "/"); i >= 0 {
		return raw[i+1:]
	}
	return raw
}

// RenderConfig produces the on-disk config.json that the mhrv-rs binary
// reads. Schema verified against mhrv-rs v1.9.21 — fields are flat and
// snake_case (mode, script_id, auth_key, listen_host, listen_port,
// socks5_port, google_ip, front_domain, log_level, verify_ssl, hosts).
func RenderConfig(c stack.RelayConfig) ([]byte, error) {
	listen := c.EffectiveListen()
	host, port, err := splitHostPort(listen)
	if err != nil {
		return nil, err
	}
	deployments := DeploymentIDs(c)
	if len(deployments) == 0 {
		return nil, fmt.Errorf("relay: at least one deployment id (or script_url) is required")
	}
	if c.AuthKey == "" {
		return nil, fmt.Errorf("relay: auth_key is required")
	}

	obj := map[string]any{
		"mode":         "apps_script",
		"auth_key":     c.AuthKey,
		"listen_host":  host,
		"listen_port":  port,
		"google_ip":    firstNonEmpty(c.GoogleIP, stack.DefaultRelayGoogleIP),
		"front_domain": firstNonEmpty(c.FrontDomain, stack.DefaultRelayFrontDomain),
		"log_level":    firstNonEmpty(c.LogLevel, stack.DefaultRelayLogLevel),
		"verify_ssl":   true,
	}
	if c.VerifySSL != nil {
		obj["verify_ssl"] = *c.VerifySSL
	}
	// script_id accepts a single string or an array; use array for >1.
	if len(deployments) == 1 {
		obj["script_id"] = deployments[0]
	} else {
		obj["script_id"] = deployments
	}
	socksPort := c.SOCKSPort
	if socksPort == 0 {
		socksPort = stack.DefaultRelaySOCKSPort
	}
	obj["socks5_port"] = socksPort

	if len(c.HostsMap) > 0 {
		obj["hosts"] = c.HostsMap
	}
	return json.MarshalIndent(obj, "", "  ")
}

// WriteConfig renders and atomically writes the relay config.json.
func WriteConfig(c stack.RelayConfig, configPath string) error {
	if configPath == "" {
		return fmt.Errorf("relay: config_path is required")
	}
	rendered, err := RenderConfig(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, append(rendered, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}

func splitHostPort(addr string) (string, int, error) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return "", 0, fmt.Errorf("invalid listen %q", addr)
	}
	host := addr[:i]
	portStr := addr[i+1:]
	if host == "" {
		host = "127.0.0.1"
	}
	if host[0] == '[' && host[len(host)-1] == ']' {
		host = host[1 : len(host)-1]
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q", portStr)
	}
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("port out of range: %d", port)
	}
	return host, port, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
