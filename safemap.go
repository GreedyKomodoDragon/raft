package raft

import (
	sync "sync"
)

type safeMap struct {
	data map[uint64]*Log
	mu   sync.RWMutex
}

func (sm *safeMap) Set(key uint64, value *Log) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.data[key] = value
}

func (sm *safeMap) Get(key uint64) (*Log, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	val, exists := sm.data[key]
	return val, exists
}

func (sm *safeMap) DeleteRange(start, finish uint64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i := start; i <= finish; i++ {
		delete(sm.data, i)
	}
}
