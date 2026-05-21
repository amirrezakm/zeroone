package api

import (
	"github.com/amirrezakm/zeroone/internal/snapshots"
)

// autoSnapshotCap bounds how many auto-source snapshots we retain on
// disk. Manual snapshots are kept indefinitely; only the auto stream is
// pruned. Sized to comfortably cover bursts of edits while keeping the
// snapshot dir from growing without bound.
const autoSnapshotCap = 50

// autoSnapshot captures a snapshot tagged source=auto with the given
// action+title, then prunes the auto stream back to autoSnapshotCap.
// All failures are non-fatal: they're recorded to the audit log so they
// don't silently mask a mutation that succeeded.
func (s *Server) autoSnapshot(actor, action, title string) {
	if s.snapshots == nil {
		return
	}
	info, err := s.snapshots.Capture(s.configPath, s.cfg.Server.XrayConfigPath, snapshots.Info{
		Title:  title,
		Source: snapshots.SourceAuto,
		Action: action,
	})
	if err != nil {
		s.recordAudit(actor, "snapshot.error", action, map[string]any{"error": err.Error()})
		return
	}
	removed, err := s.snapshots.Prune(autoSnapshotCap)
	if err != nil {
		s.recordAudit(actor, "snapshot.prune.error", info.ID, map[string]any{"error": err.Error()})
		return
	}
	if len(removed) > 0 {
		s.recordAudit("system", "snapshot.prune", "", map[string]any{"removed": removed, "cap": autoSnapshotCap})
	}
}
