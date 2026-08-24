package execution

import (
	"sync"
	"time"
)

type Store struct {
	mu               sync.RWMutex
	records          map[string]*ExecutionRecord
	idempotencyKeys  map[string]string // key (planId_actionId) -> executionId
	retentionSeconds int
}

func NewStore(retentionSeconds int) *Store {
	return &Store{
		records:          make(map[string]*ExecutionRecord),
		idempotencyKeys:  make(map[string]string),
		retentionSeconds: retentionSeconds,
	}
}

func (s *Store) Save(record *ExecutionRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.ExecutionID] = record
	idempotencyKey := record.PlanID + "_" + record.ActionID
	s.idempotencyKeys[idempotencyKey] = record.ExecutionID
}

func (s *Store) Get(executionID string) (*ExecutionRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[executionID]
	if !ok {
		return nil, false
	}
	// return a copy
	cp := *record
	return &cp, true
}

func (s *Store) GetByIdempotencyKey(planID, actionID string) (*ExecutionRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := planID + "_" + actionID
	execID, ok := s.idempotencyKeys[key]
	if !ok {
		return nil, false
	}
	return s.Get(execID)
}

func (s *Store) GetByIncident(incidentID string) []*ExecutionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var results []*ExecutionRecord
	for _, r := range s.records {
		if r.IncidentID == incidentID {
			cp := *r
			results = append(results, &cp)
		}
	}
	return results
}

func (s *Store) Cleanup(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := now.Add(-time.Duration(s.retentionSeconds) * time.Second)
	for id, r := range s.records {
		if r.StartedAt.Before(cutoff) {
			delete(s.records, id)
			idemKey := r.PlanID + "_" + r.ActionID
			delete(s.idempotencyKeys, idemKey)
		}
	}
}
