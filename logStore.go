package raft

import "fmt"

type Log struct {
	Term           uint64
	Index          uint64
	LogType        LogType
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
	logs  []Log
	index uint64
	term  uint64
}

func NewLogStore() LogStore {
	return &logStore{
		logs:  []Log{},
		index: 1,
		term:  0,
	}
}

func (l *logStore) AppendLog(log Log) error {
	if l.logs == nil {
		return fmt.Errorf("missing slice")
	}

	l.logs = append(l.logs, log)
	return nil
}

func (l *logStore) GetLog(index uint64) (*Log, error) {
	if l.logs == nil {
		return nil, fmt.Errorf("missing slice")
	}

	if uint64(len(l.logs)) < index {
		return nil, fmt.Errorf("GetLog: missing index", len(l.logs), index)
	}

	return &l.logs[index-1], nil
}

func (l *logStore) UpdateCommited(index uint64) error {
	if l.logs == nil {
		return fmt.Errorf("missing slice")
	}

	if len(l.logs) < int(index) {
		return fmt.Errorf("UpdateCommited: missing index")
	}

	l.logs[index-1].LeaderCommited = true
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
