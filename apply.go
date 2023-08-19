package raft

// Using
type LogType []byte

var (
	DATA_LOG LogType = []byte{0}
)

type CommitedLog struct {
	Index uint64
	Type  LogType
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
