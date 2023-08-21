package raft

import (
	"fmt"
	"sync"
)

type Log struct {
	Term           uint64
	Index          uint64
	LogType        uint64
	Data           []byte
	LeaderCommited bool
}

type LogStore interface {
	AppendLog(Log) error
	GetLog(uint64) (*Log, error)
	UpdateCommited(uint64) error
	IncrementIndex()
	IncrementTerm()
	GetLatestIndex() uint64
	GetLatestTerm() uint64
}

type logStore struct {
	logs  *safeMap
	index uint64
	term  uint64
}

func NewLogStore() LogStore {
	return &logStore{
		logs: &safeMap{
			make(map[uint64]*Log),
			sync.RWMutex{},
		},
		index: 1,
		term:  0,
	}
}

func (l *logStore) AppendLog(log Log) error {
	if l.logs == nil {
		return fmt.Errorf("missing slice")
	}

	l.logs.Set(log.Index, &log)
	return nil
}

func (l *logStore) GetLog(index uint64) (*Log, error) {
	if l.logs == nil {
		return nil, fmt.Errorf("missing slice")
	}

	log, ok := l.logs.Get(index)
	if !ok {
		return nil, fmt.Errorf("cannot find log", index)
	}

	return log, nil
}

func (l *logStore) UpdateCommited(index uint64) error {
	if l.logs == nil {
		return fmt.Errorf("missing slice")
	}

	log, ok := l.logs.Get(index)
	if !ok {
		return fmt.Errorf("cannot find log: %v", index)
	}

	log.LeaderCommited = true
	return nil
}

func (l *logStore) IncrementIndex() {
	l.index++
}

func (l *logStore) IncrementTerm() {
	l.term++
}

func (l *logStore) GetLatestIndex() uint64 {
	return l.index
}

func (l *logStore) GetLatestTerm() uint64 {
	return l.term
}
