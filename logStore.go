package raft

import "fmt"

type Log struct {
	Term    uint64
	LogType []byte
	Data    []byte
}

type LogStore interface {
	AppendLog(Log) error
}

type logStore struct {
	logs map[uint64]Log
}

func NewLogStore() LogStore {
	return &logStore{
		logs: map[uint64]Log{},
	}
}

func (l *logStore) AppendLog(log Log) error {
	if l.logs == nil {
		return fmt.Errorf("missing map")
	}

	l.logs[log.Term] = log
	return nil
}
