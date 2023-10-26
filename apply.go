package raft

const (
	DATA_LOG uint64 = iota
	RAFT_LOG
)

type ApplicationApply interface {
	Apply(Log) (interface{}, error)
}

type StdOutApply struct {
}

func (s *StdOutApply) Apply(log Log) (interface{}, error) {
	// fmt.Println("log applied:", log.Index, string(log.Data))
	return nil, nil
}
