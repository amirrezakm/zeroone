// SPDX-License-Identifier: AGPL-3.0-or-later
package xrayinstall

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State is the on-disk record of the most recent override install. It
// lives at Root/state.json. Absence of the file means "no override
// has ever been applied" — i.e. the daemon is running the image binary.
type State struct {
	InstalledVersion string `json:"installed_version,omitempty"`
	InstalledAt      int64  `json:"installed_at,omitempty"`
	Source           string `json:"source,omitempty"` // "online" | "upload"
	BinarySHA256     string `json:"binary_sha256,omitempty"`
	GeoIPSHA256      string `json:"geoip_sha256,omitempty"`
	GeoSiteSHA256    string `json:"geosite_sha256,omitempty"`
	LastCheckUnix    int64  `json:"last_check,omitempty"`
	LastCheckLatest  string `json:"last_check_latest,omitempty"`
	PreviousVersion  string `json:"previous_version,omitempty"` // for rollback breadcrumb
}

func (i *Installer) statePath() string { return filepath.Join(i.Root, "state.json") }

// LoadState reads state.json. A missing file is not an error — it
// returns a zero State, which the caller interprets as "no override".
func (i *Installer) LoadState() (State, error) {
	b, err := os.ReadFile(i.statePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("xrayinstall: read state: %w", err)
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return State{}, fmt.Errorf("xrayinstall: parse state: %w", err)
	}
	return st, nil
}

// SaveState writes state.json via tmp + rename so a crash never leaves
// a half-written file. Best-effort fsync is intentionally skipped; the
// daemon does not have a strict durability contract here.
func (i *Installer) SaveState(st State) error {
	if err := i.EnsureDirs(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := i.statePath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, i.statePath())
}

// touchLastCheck stamps the cache time + latest version under a single
// lock acquisition. Used by CheckLatest so the panel can show "last
// checked: 30s ago" without an extra round-trip.
func (i *Installer) touchLastCheck(latest string) {
	st, err := i.LoadState()
	if err != nil {
		return
	}
	st.LastCheckUnix = time.Now().Unix()
	st.LastCheckLatest = latest
	_ = i.SaveState(st)
}
