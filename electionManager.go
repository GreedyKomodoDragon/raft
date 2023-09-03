package raft

import (
	"math/rand"
	"time"
)

type electionManager struct {
	currentState  role
	foundLeader   bool
	votes         uint64
	clientedVoted uint64
	electionTimer *time.Timer
	logStore      LogStore
}

func newElectionManager(logStore LogStore) *electionManager {
	return &electionManager{
		currentState:  FOLLOWER,
		votes:         0,
		clientedVoted: 0,
		logStore:      logStore,
		electionTimer: time.NewTimer(time.Millisecond * time.Duration(3000+rand.Intn(6000))),
		foundLeader:   false,
	}
}

func (e *electionManager) resetVotes() {
	e.votes = 1
	e.clientedVoted = 1
}
