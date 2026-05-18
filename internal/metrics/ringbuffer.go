// Package metrics is a small in-memory time-series store for the panel.
// Two resolutions are kept: 5s buckets for the last hour and 1m buckets
// for the last 24 hours. Samples beyond that age out automatically.
package metrics

import (
	"sync"
	"time"
)

// Sample is a single point. Fields are float64 so charts can plot
// percentages, byte counters, and rates without conversion.
type Sample struct {
	Timestamp int64              `json:"t"`
	Values    map[string]float64 `json:"v"`
}

// Series wraps a fixed-capacity ring of samples.
type Series struct {
	mu       sync.RWMutex
	cap      int
	step     time.Duration
	samples  []Sample
	writeIdx int
	count    int
}

func NewSeries(capacity int, step time.Duration) *Series {
	return &Series{cap: capacity, step: step, samples: make([]Sample, capacity)}
}

func (s *Series) Append(sample Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples[s.writeIdx] = sample
	s.writeIdx = (s.writeIdx + 1) % s.cap
	if s.count < s.cap {
		s.count++
	}
}

// Snapshot returns the samples in chronological order, newest last.
// since limits to samples newer than the given unix-second timestamp.
func (s *Series) Snapshot(since int64) []Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Sample, 0, s.count)
	start := s.writeIdx - s.count
	if start < 0 {
		start += s.cap
	}
	for i := 0; i < s.count; i++ {
		idx := (start + i) % s.cap
		sample := s.samples[idx]
		if sample.Timestamp >= since {
			out = append(out, sample)
		}
	}
	return out
}

// Step returns the bucket size of this series.
func (s *Series) Step() time.Duration { return s.step }
