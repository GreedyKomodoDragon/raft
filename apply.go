package raft

import "fmt"

const (
	DATA_LOG uint64 = iota
	RAFT_LOG
)

type ApplicationApply interface {
	Apply(Log) ([]byte, error)
}

type StdOutApply struct {
}

func (s *StdOutApply) Apply(log Log) ([]byte, error) {
	fmt.Println("log applied:", log.Index)
	return nil, nil
}
