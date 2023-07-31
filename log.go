package raft

// Use protobuf for this
type Log struct {
	Term    uint64
	LogType uint64
	Data    []byte
}
