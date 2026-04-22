package core

import "sync"

// MapStateStore is a mutex-backed in-memory StateStore.
type MapStateStore struct {
	mu sync.RWMutex
	m  map[ProcessID]ProcessState
}

// NewMapStateStore returns an empty store.
func NewMapStateStore() *MapStateStore {
	return &MapStateStore{m: make(map[ProcessID]ProcessState)}
}

func (s *MapStateStore) Set(id ProcessID, st ProcessState) {
	s.mu.Lock()
	s.m[id] = st
	s.mu.Unlock()
}

func (s *MapStateStore) Get(id ProcessID) (ProcessState, bool) {
	s.mu.RLock()
	st, ok := s.m[id]
	s.mu.RUnlock()
	return st, ok
}

func (s *MapStateStore) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[ProcessID]ProcessState, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return Snapshot{Processes: out}
}

func (s *MapStateStore) Delete(id ProcessID) {
	s.mu.Lock()
	delete(s.m, id)
	s.mu.Unlock()
}
