package raft

type Raft interface {
	Start()
}

type raft struct {
	server []Server
	logs   []Log
}

func NewRaftServer(server []Server) Raft {
	return &raft{
		server: server,
		logs:   []Log{},
	}
}

func (r *raft) Start() {

}
