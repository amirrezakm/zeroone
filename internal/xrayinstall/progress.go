// SPDX-License-Identifier: AGPL-3.0-or-later
package xrayinstall

import (
	"sync/atomic"
	"time"
)

// JobPhase enumerates the lifecycle steps a single update walks
// through. Surfaced in the panel as a progress label.
const (
	PhaseQueued      = "queued"
	PhaseDownloading = "downloading"
	PhaseVerifying   = "verifying"
	PhaseStaging     = "staging"
	PhaseSwapping    = "swapping"
	PhaseRestarting  = "restarting"
	PhaseDone        = "done"
	PhaseFailed      = "failed"
)

// Job is a single panel-triggered update operation. Only one job runs
// at a time; the installer serialises overlapping requests via a
// 409-equivalent error.
//
// BytesDone is written by the download goroutine via atomic.AddInt64
// and read by panel polls via atomic.LoadInt64 — no embedded
// noCopy-tainted type, so the value can be copied across boundaries
// without tripping go vet.
type Job struct {
	ID            string `json:"id"`
	Phase         string `json:"phase"`
	StartedAt     int64  `json:"started_at"`
	FinishedAt    int64  `json:"finished_at,omitempty"`
	BytesTotal    int64  `json:"bytes_total,omitempty"`
	BytesDone     int64  `json:"bytes_done,omitempty"`
	TargetVersion string `json:"target_version,omitempty"`
	Source        string `json:"source,omitempty"` // "online" | "upload"
	Error         string `json:"error,omitempty"`
}

// snapshot returns a stable copy of the current job state. Atomic
// reads ensure BytesDone is internally consistent with the rest of the
// fields even when the download goroutine is still ticking.
func (j *Job) snapshot() Job {
	if j == nil {
		return Job{}
	}
	out := *j
	out.BytesDone = atomic.LoadInt64(&j.BytesDone)
	return out
}

func (j *Job) setPhase(p string) {
	if j == nil {
		return
	}
	j.Phase = p
}

func (j *Job) addBytes(n int64) {
	if j == nil {
		return
	}
	atomic.AddInt64(&j.BytesDone, n)
}

func (j *Job) finish(phase, errMsg string) {
	if j == nil {
		return
	}
	j.Phase = phase
	j.FinishedAt = time.Now().Unix()
	j.Error = errMsg
}
