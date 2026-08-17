package aireasoning

import (
	"sync"
	"time"
)

// Store provides bounded in-memory storage for AI analyses.
type Store struct {
	mu               sync.RWMutex
	analyses         map[string]*AnalysisResult // keyed by incidentID
	retentionSeconds int
}

func NewStore(retentionSeconds int) *Store {
	if retentionSeconds <= 0 {
		retentionSeconds = 3600 // default 1 hour
	}
	return &Store{
		analyses:         make(map[string]*AnalysisResult),
		retentionSeconds: retentionSeconds,
	}
}

func (s *Store) Save(analysis *AnalysisResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if analysis == nil || analysis.IncidentID == "" {
		return
	}
	
	s.analyses[analysis.IncidentID] = analysis
}

func (s *Store) Get(incidentID string) (*AnalysisResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	a, ok := s.analyses[incidentID]
	if !ok {
		return nil, false
	}
	
	// Create a safe copy
	aCopy := *a
	return &aCopy, true
}

func (s *Store) Cleanup(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := now.Add(-time.Duration(s.retentionSeconds) * time.Second)
	
	for id, a := range s.analyses {
		if a.GeneratedAt.Before(cutoff) {
			delete(s.analyses, id)
		}
	}
}
