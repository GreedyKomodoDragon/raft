package raft

// Using

const (
	DATA_LOG uint64 = iota
	RAFT_LOG
)

type CommitedLog struct {
	Index uint64
	Type  uint64
	Data  []byte
}

type ApplicationApply interface {
	Apply(CommitedLog) ([]byte, error)
}

type StdOutApply struct {
}

func (s *StdOutApply) Apply(log CommitedLog) ([]byte, error) {
	// fmt.Println("log applied:", log.Index, string(log.Data))
	return nil, nil
}
