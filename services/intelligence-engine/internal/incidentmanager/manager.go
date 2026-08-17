package incidentmanager

import (
	"fmt"
	"sync"
	"time"

	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/google/uuid"
)

type Config struct {
	RecoverySeconds  time.Duration
	RetentionSeconds time.Duration
}

func DefaultConfig() Config {
	return Config{
		RecoverySeconds:  30 * time.Second,
		RetentionSeconds: 3600 * time.Second,
	}
}

type Manager struct {
	cfg       Config
	incidents map[string]*incidentmodel.Incident // key: IncidentID
	evStore   *evidence.Store
	mu        sync.RWMutex
}

func NewManager(cfg Config, evStore *evidence.Store) *Manager {
	return &Manager{
		cfg:       cfg,
		incidents: make(map[string]*incidentmodel.Incident),
		evStore:   evStore,
	}
}

func getFingerprint(sig incidentsignal.Signal) string {
	return fmt.Sprintf("%s|%s|%s", sig.Service, sig.Operation, sig.Type)
}

func getSeverity(sig incidentsignal.Signal) incidentmodel.Severity {
	if sig.Type == incidentsignal.SignalTypeErrorRate && sig.Value > 0.50 {
		return incidentmodel.SeverityCritical
	}
	if sig.Type == incidentsignal.SignalTypeDependencyFailure {
		return incidentmodel.SeverityCritical
	}
	return incidentmodel.SeverityWarning
}

func (m *Manager) ProcessSignal(sig incidentsignal.Signal) {
	fingerprint := getFingerprint(sig)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Try to find an OPEN incident with this fingerprint
	var existing *incidentmodel.Incident
	for _, inc := range m.incidents {
		if inc.Status == incidentmodel.StatusOpen && inc.Fingerprint == fingerprint {
			existing = inc
			break
		}
	}

	// Store evidence globally
	m.evStore.Add(sig.Evidence)

	if existing != nil {
		existing.LastUpdatedAt = sig.Timestamp
		// Add traceID if new
		if sig.TraceID != "" {
			found := false
			for _, tid := range existing.TraceIDs {
				if tid == sig.TraceID {
					found = true
					break
				}
			}
			if !found {
				existing.TraceIDs = append(existing.TraceIDs, sig.TraceID)
			}
		}
		// Add evidence if new
		existing.EvidenceIDs = append(existing.EvidenceIDs, sig.Evidence.EvidenceID)
		
		// Upgrade severity if needed
		newSev := getSeverity(sig)
		if newSev == incidentmodel.SeverityCritical && existing.Severity != incidentmodel.SeverityCritical {
			existing.Severity = incidentmodel.SeverityCritical
		}
	} else {
		// Create new incident
		incID := uuid.New().String()
		title := fmt.Sprintf("%s degradation on %s", sig.Service, sig.Operation)
		
		inc := &incidentmodel.Incident{
			IncidentID:      incID,
			Status:          incidentmodel.StatusOpen,
			Severity:        getSeverity(sig),
			Title:           title,
			Description:     sig.Evidence.Description,
			StartedAt:       sig.Timestamp,
			LastUpdatedAt:   sig.Timestamp,
			Fingerprint:     fingerprint,
			RootService:     sig.Service,
			RootOperation:   sig.Operation,
			AffectedServices: []string{sig.Service},
			TraceIDs:        []string{},
			EvidenceIDs:     []string{sig.Evidence.EvidenceID},
			DetectionReason: sig.Evidence.Description,
		}
		if sig.TraceID != "" {
			inc.TraceIDs = append(inc.TraceIDs, sig.TraceID)
		}
		m.incidents[incID] = inc
	}
}

func (m *Manager) CleanupAndResolve() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, inc := range m.incidents {
		if inc.Status == incidentmodel.StatusOpen {
			if now.Sub(inc.LastUpdatedAt) > m.cfg.RecoverySeconds {
				inc.Status = incidentmodel.StatusResolved
				t := inc.LastUpdatedAt.Add(m.cfg.RecoverySeconds)
				inc.ResolvedAt = &t
			}
		} else if inc.Status == incidentmodel.StatusResolved {
			if inc.ResolvedAt != nil && now.Sub(*inc.ResolvedAt) > m.cfg.RetentionSeconds {
				delete(m.incidents, id)
			}
		}
	}
}

func (m *Manager) GetOpenIncidents() []*incidentmodel.Incident {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*incidentmodel.Incident
	for _, inc := range m.incidents {
		if inc.Status == incidentmodel.StatusOpen {
			res = append(res, cloneIncident(inc))
		}
	}
	return res
}

func (m *Manager) GetAllIncidents() []*incidentmodel.Incident {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*incidentmodel.Incident
	for _, inc := range m.incidents {
		res = append(res, cloneIncident(inc))
	}
	return res
}

func (m *Manager) GetIncident(id string) *incidentmodel.Incident {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inc, ok := m.incidents[id]
	if !ok {
		return nil
	}
	return cloneIncident(inc)
}

func (m *Manager) UpdateIncident(inc *incidentmodel.Incident) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Simply replace
	if _, ok := m.incidents[inc.IncidentID]; ok {
		m.incidents[inc.IncidentID] = inc
	}
}

func cloneIncident(inc *incidentmodel.Incident) *incidentmodel.Incident {
	cloned := *inc
	cloned.AffectedServices = append([]string(nil), inc.AffectedServices...)
	cloned.AffectedOperations = append([]string(nil), inc.AffectedOperations...)
	cloned.AffectedEdges = append([]string(nil), inc.AffectedEdges...)
	cloned.TraceIDs = append([]string(nil), inc.TraceIDs...)
	cloned.EvidenceIDs = append([]string(nil), inc.EvidenceIDs...)
	if inc.RCA != nil {
		r := *inc.RCA
		cloned.RCA = &r
	}
	if inc.ResolvedAt != nil {
		t := *inc.ResolvedAt
		cloned.ResolvedAt = &t
	}
	return &cloned
}
