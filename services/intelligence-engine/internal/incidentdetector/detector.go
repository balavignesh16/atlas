package incidentdetector

import (
	"fmt"
	"sync"
	"time"

	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/window"
)

type Config struct {
	ErrorRateThreshold          float64
	LatencyThresholdMs          float64
	DependencyErrorRateThreshold float64
	IncidentWindowSeconds       time.Duration
	MinObservations             int
}

func DefaultConfig() Config {
	return Config{
		ErrorRateThreshold:          0.20,
		LatencyThresholdMs:          1000.0,
		DependencyErrorRateThreshold: 0.20,
		IncidentWindowSeconds:       60 * time.Second,
		MinObservations:             10,
	}
}

type Detector struct {
	cfg          Config
	graphEngine  *graph.DependencyGraph
	windows      map[string]*window.Window // key: "service|operation"
	windowsMu    sync.RWMutex
	signalsChan  chan<- incidentsignal.Signal
}

func NewDetector(cfg Config, graphEngine *graph.DependencyGraph, signalsChan chan<- incidentsignal.Signal) *Detector {
	return &Detector{
		cfg:         cfg,
		graphEngine: graphEngine,
		windows:     make(map[string]*window.Window),
		signalsChan: signalsChan,
	}
}

func getWindowKey(service, operation string) string {
	return fmt.Sprintf("%s|%s", service, operation)
}

func (d *Detector) getWindow(service, operation string) *window.Window {
	key := getWindowKey(service, operation)
	d.windowsMu.RLock()
	w, ok := d.windows[key]
	d.windowsMu.RUnlock()

	if ok {
		return w
	}

	d.windowsMu.Lock()
	defer d.windowsMu.Unlock()
	// double check
	w, ok = d.windows[key]
	if ok {
		return w
	}
	w = window.NewWindow()
	d.windows[key] = w
	return w
}

func (d *Detector) ProcessEvent(e event.ATLASEvent) {
	if e.EventType != "SPAN" {
		return
	}
	w := d.getWindow(e.ServiceName, e.OperationName)
	isError := e.Status == "ERROR" || e.Status == "5xx"
	w.Add(float64(e.DurationMs), isError, e.Timestamp)
}

func (d *Detector) EvaluateAll() {
	now := time.Now()
	thresholdTime := now.Add(-d.cfg.IncidentWindowSeconds)

	d.windowsMu.RLock()
	// Copy keys and windows to avoid holding the global lock during evaluation
	windowsToEvaluate := make(map[string]*window.Window, len(d.windows))
	for k, w := range d.windows {
		windowsToEvaluate[k] = w
	}
	d.windowsMu.RUnlock()

	// 1. Evaluate service windows (Error Rate and Latency)
	for key, w := range windowsToEvaluate {
		w.CleanupExpired(thresholdTime)
		
		count := w.Count()
		if count < d.cfg.MinObservations {
			continue // Avoid single-request false positives
		}

		// Extract service and operation from key
		// key is "service|operation"
		// For signals, we can just split it
		// But let's just do it directly in rules
		d.evaluateWindowRules(key, w, now)
	}

	// 2. Evaluate dependencies (Dependency Failure)
	d.evaluateDependencies(now)
}

func (d *Detector) emitSignal(sig incidentsignal.Signal) {
	select {
	case d.signalsChan <- sig:
	default:
		// If channel is full, drop it or log it. For now, non-blocking send.
	}
}
