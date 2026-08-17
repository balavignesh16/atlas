package evidence

import (
	"sync"
	"time"
)

type Store struct {
	mu       sync.RWMutex
	evidence map[string]Evidence // key: EvidenceID
}

func NewStore() *Store {
	return &Store{
		evidence: make(map[string]Evidence),
	}
}

func (s *Store) Add(ev Evidence) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidence[ev.EvidenceID] = ev
}

func (s *Store) Get(id string) (Evidence, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ev, ok := s.evidence[id]
	return ev, ok
}

func (s *Store) GetAll(ids []string) []Evidence {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var res []Evidence
	for _, id := range ids {
		if ev, ok := s.evidence[id]; ok {
			res = append(res, ev)
		}
	}
	return res
}

func (s *Store) CleanupExpired(retention time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, ev := range s.evidence {
		if now.Sub(ev.Timestamp) > retention {
			delete(s.evidence, id)
		}
	}
}
