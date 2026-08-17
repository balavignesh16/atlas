package incidentdetector

import (
	"fmt"
	"strings"
	"time"

	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/window"
	"github.com/google/uuid"
)

func (d *Detector) evaluateWindowRules(key string, w *window.Window, now time.Time) {
	parts := strings.SplitN(key, "|", 2)
	if len(parts) != 2 {
		return
	}
	service := parts[0]
	operation := parts[1]

	errorRate := w.ErrorRate()
	if errorRate > d.cfg.ErrorRateThreshold {
		sig := incidentsignal.Signal{
			SignalID:  uuid.New().String(),
			Type:      incidentsignal.SignalTypeErrorRate,
			Timestamp: now,
			Service:   service,
			Operation: operation,
			Value:     errorRate,
			Threshold: d.cfg.ErrorRateThreshold,
			Direction: "GREATER_THAN",
			Evidence: evidence.Evidence{
				EvidenceID:  uuid.New().String(),
				Type:        evidence.EvidenceTypeErrorRate,
				Timestamp:   now,
				Service:     service,
				Operation:   operation,
				Description: fmt.Sprintf("Error rate %.2f%% exceeded threshold %.2f%%", errorRate*100, d.cfg.ErrorRateThreshold*100),
				Value:       errorRate,
				Expected:    d.cfg.ErrorRateThreshold,
				Observed:    errorRate,
				Source:      "IncidentDetector",
			},
		}
		d.emitSignal(sig)
	}

	avgLatency := w.AverageLatency()
	if avgLatency > d.cfg.LatencyThresholdMs {
		sig := incidentsignal.Signal{
			SignalID:  uuid.New().String(),
			Type:      incidentsignal.SignalTypeLatency,
			Timestamp: now,
			Service:   service,
			Operation: operation,
			Value:     avgLatency,
			Threshold: d.cfg.LatencyThresholdMs,
			Direction: "GREATER_THAN",
			Evidence: evidence.Evidence{
				EvidenceID:  uuid.New().String(),
				Type:        evidence.EvidenceTypeLatency,
				Timestamp:   now,
				Service:     service,
				Operation:   operation,
				Description: fmt.Sprintf("Average latency %.2fms exceeded threshold %.2fms", avgLatency, d.cfg.LatencyThresholdMs),
				Value:       avgLatency,
				Expected:    d.cfg.LatencyThresholdMs,
				Observed:    avgLatency,
				Source:      "IncidentDetector",
			},
		}
		d.emitSignal(sig)
	}
}

func (d *Detector) evaluateDependencies(now time.Time) {
	edges := d.graphEngine.GetEdges()
	for _, edge := range edges {
		if edge.CallCount < int64(d.cfg.MinObservations) {
			continue
		}
		errRate := float64(edge.ErrorCount) / float64(edge.CallCount)
		if errRate > d.cfg.DependencyErrorRateThreshold {
			sig := incidentsignal.Signal{
				SignalID:  uuid.New().String(),
				Type:      incidentsignal.SignalTypeDependencyFailure,
				Timestamp: now,
				Service:   edge.SourceService,
				Operation: fmt.Sprintf("Call to %s", edge.TargetService),
				Value:     errRate,
				Threshold: d.cfg.DependencyErrorRateThreshold,
				Direction: "GREATER_THAN",
				Evidence: evidence.Evidence{
					EvidenceID:  uuid.New().String(),
					Type:        evidence.EvidenceTypeDependencyError,
					Timestamp:   now,
					Service:     edge.SourceService,
					Operation:   fmt.Sprintf("Call to %s", edge.TargetService),
					Description: fmt.Sprintf("Dependency %s -> %s error rate %.2f%% exceeded %.2f%%", edge.SourceService, edge.TargetService, errRate*100, d.cfg.DependencyErrorRateThreshold*100),
					Value:       errRate,
					Expected:    d.cfg.DependencyErrorRateThreshold,
					Observed:    errRate,
					Source:      "IncidentDetector",
				},
			}
			d.emitSignal(sig)
		}
	}
}
