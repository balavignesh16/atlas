package remediation

import (
	"sync"
	"time"
)

// Store provides bounded in-memory storage for remediation plans.
type Store struct {
	mu               sync.RWMutex
	plans            map[string]*RemediationPlan // keyed by planId
	incidentMap      map[string]string           // incidentId -> planId
	retentionSeconds int
}

func NewStore(retentionSeconds int) *Store {
	if retentionSeconds <= 0 {
		retentionSeconds = 3600 // default 1 hour
	}
	return &Store{
		plans:            make(map[string]*RemediationPlan),
		incidentMap:      make(map[string]string),
		retentionSeconds: retentionSeconds,
	}
}

func (s *Store) Save(plan *RemediationPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if plan == nil || plan.PlanID == "" {
		return
	}
	
	s.plans[plan.PlanID] = plan
	s.incidentMap[plan.IncidentID] = plan.PlanID
}

func (s *Store) Get(planID string) (*RemediationPlan, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	p, ok := s.plans[planID]
	if !ok {
		return nil, false
	}
	
	// Safe copy
	pCopy := *p
	return &pCopy, true
}

func (s *Store) GetByIncident(incidentID string) (*RemediationPlan, bool) {
	s.mu.RLock()
	planID, ok := s.incidentMap[incidentID]
	s.mu.RUnlock()
	
	if !ok {
		return nil, false
	}
	return s.Get(planID)
}

func (s *Store) Cleanup(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := now.Add(-time.Duration(s.retentionSeconds) * time.Second)
	
	for id, p := range s.plans {
		if p.CreatedAt.Before(cutoff) {
			delete(s.incidentMap, p.IncidentID)
			delete(s.plans, id)
		}
	}
}
