package snispoof

import (
	"errors"
	"io"
	"os"

	"github.com/amirrezakm/zeroone/internal/stack"
)

// Tail returns the last N lines of the plugin log file (best-effort). Both
// byedpi and tun2socks append to the same file.
func Tail(path string, max int) ([]string, error) {
	if path == "" {
		path = stack.DefaultSNISpoofLogPath
	}
	if max <= 0 {
		max = 200
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()
	const window = 256 * 1024
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var off int64
	if size := st.Size(); size > window {
		off = size - window
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil, err
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	lines := splitLines(string(buf))
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return lines, nil
}

func splitLines(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
