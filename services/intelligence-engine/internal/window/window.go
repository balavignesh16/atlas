package window

import (
	"sort"
	"sync"
	"time"
)

type Observation struct {
	Timestamp  time.Time
	DurationMs float64
	IsError    bool
	TraceID    string
}

type Window struct {
	mu           sync.RWMutex
	observations []Observation
}

func NewWindow() *Window {
	return &Window{
		observations: make([]Observation, 0, 100),
	}
}

// Add records an observation. traceID is optional (may be empty) -- it is
// carried purely as additional RCA evidence (see RecentTraceID); a missing
// traceID never affects detection, error rate, or latency calculations.
func (w *Window) Add(durationMs float64, isError bool, timestamp time.Time, traceID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.observations = append(w.observations, Observation{
		Timestamp:  timestamp,
		DurationMs: durationMs,
		IsError:    isError,
		TraceID:    traceID,
	})
}

// RecentTraceID returns the trace ID of the most recent observation that
// has one, or "" if the window is empty or none of its observations carry a
// trace ID. This is best-effort additional evidence for RCA's temporal
// precedence check -- callers must treat an empty result as "no trace
// evidence available" and continue exactly as before, never as an error.
func (w *Window) RecentTraceID() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for i := len(w.observations) - 1; i >= 0; i-- {
		if w.observations[i].TraceID != "" {
			return w.observations[i].TraceID
		}
	}
	return ""
}

func (w *Window) CleanupExpired(threshold time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Find the index of the first observation that is AT OR AFTER the threshold
	// Since observations are typically added in chronological order (or close to it),
	// but out-of-order events might exist, we should probably do a filtering approach to be safe.
	// But assuming ordered appends, a binary search could work.
	// For safety, let's just do a full filter since window sizes are bounded (60s).
	var active []Observation
	for _, obs := range w.observations {
		if obs.Timestamp.After(threshold) || obs.Timestamp.Equal(threshold) {
			active = append(active, obs)
		}
	}
	w.observations = active
}

func (w *Window) Count() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.observations)
}

func (w *Window) ErrorCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	count := 0
	for _, obs := range w.observations {
		if obs.IsError {
			count++
		}
	}
	return count
}

func (w *Window) ErrorRate() float64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if len(w.observations) == 0 {
		return 0.0
	}
	errCount := 0
	for _, obs := range w.observations {
		if obs.IsError {
			errCount++
		}
	}
	return float64(errCount) / float64(len(w.observations))
}

func (w *Window) AverageLatency() float64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if len(w.observations) == 0 {
		return 0.0
	}
	sum := 0.0
	for _, obs := range w.observations {
		sum += obs.DurationMs
	}
	return sum / float64(len(w.observations))
}

func (w *Window) MaxLatency() float64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if len(w.observations) == 0 {
		return 0.0
	}
	max := 0.0
	for _, obs := range w.observations {
		if obs.DurationMs > max {
			max = obs.DurationMs
		}
	}
	return max
}

// getPercentile is an internal helper. Assumes w.mu is held.
// Sorts a copy of the latencies to find the exact percentile.
func (w *Window) getPercentile(p float64) float64 {
	if len(w.observations) == 0 {
		return 0.0
	}
	latencies := make([]float64, len(w.observations))
	for i, obs := range w.observations {
		latencies[i] = obs.DurationMs
	}
	sort.Float64s(latencies)

	index := int(float64(len(latencies)-1) * p)
	return latencies[index]
}

func (w *Window) P95Latency() float64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.getPercentile(0.95)
}

func (w *Window) P99Latency() float64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.getPercentile(0.99)
}
