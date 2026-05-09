package failover

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const DefaultStatePath = "/var/lib/xray-stack/failover-state.json"

func LoadState(path string) (State, error) {
	if path == "" {
		path = DefaultStatePath
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return State{}, err
	}
	return st, nil
}

func SaveState(path string, st State) error {
	if path == "" {
		path = DefaultStatePath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
