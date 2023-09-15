package raft

import (
	"math/rand"
	"time"

	"github.com/rs/zerolog/log"
)

type electionManager struct {
	currentState  role
	foundLeader   bool
	votes         uint64
	clientedVoted uint64
	electionTimer *time.Timer
	logStore      LogStore

	voteReceived        chan *SendVoteRequest
	heartBeatTime       time.Time
	heartBeatTimeDouble time.Duration
	heartBeatChan       chan time.Time

	leaderChanInternal chan interface{}
	broadcastChan      chan interface{}

	clientHalf uint64
	conf       *ElectionConfig
}

func newElectionManager(logStore LogStore, clientCount int, conf *ElectionConfig) *electionManager {

	return &electionManager{
		currentState:        FOLLOWER,
		votes:               0,
		clientedVoted:       0,
		logStore:            logStore,
		electionTimer:       time.NewTimer(time.Millisecond * time.Duration(3000+rand.Intn(6000))),
		foundLeader:         false,
		clientHalf:          uint64(clientCount/2 + 1),
		heartBeatTime:       time.Unix(0, 0),
		voteReceived:        make(chan *SendVoteRequest, clientCount),
		heartBeatChan:       make(chan time.Time, clientCount),
		leaderChanInternal:  make(chan interface{}, 1),
		broadcastChan:       make(chan interface{}, 1),
		heartBeatTimeDouble: conf.HeartbeatTimeout * 2,
		conf:                conf,
	}
}

func (e *electionManager) resetVotes() {
	e.votes = 1
	e.clientedVoted = 1
}

func (e *electionManager) start() {
	for {
		select {
		case t := <-e.heartBeatChan:
			e.heartBeatTime = t
			e.foundLeader = true

			if e.currentState == CANDIDATE {
				log.Info().Str("currentState", "CANDIDATE").Msg("demoted to follower as found leader")
				e.currentState = FOLLOWER

			}

		case event := <-e.voteReceived:
			if e.currentState == LEADER {
				log.Info().Msg("already leader so ignored")
				continue
			}

			// if found another leader
			if e.hasRecievedHeartbeat() || e.currentState == FOLLOWER {
				log.Info().Str("currentState", "CANDIDATE").Msg("demoted to follower as found leader")
				e.currentState = FOLLOWER
				continue
			}

			if event.Voted {
				log.Debug().Uint64("nodeId", event.Id).Msg("vote found")
				e.votes++
			}

			e.clientedVoted++
			if e.clientedVoted < e.clientHalf {
				log.Debug().Msg("not enough votes found")
				continue
			}

			if e.votes < e.clientHalf {
				log.Info().Msg("not enough votes in favour, demoted to follower")
				e.currentState = FOLLOWER
				continue
			}

			e.currentState = LEADER
			log.Info().Msg("became leader")
			e.leaderChanInternal <- nil

		case <-e.electionTimer.C:
			// only begin election if no heartbeat has started
			if e.hasRecievedHeartbeat() {
				e.electionTimer.Reset(e.conf.ElectionTimeout)
				continue
			}

			if e.currentState == LEADER {
				continue
			}

			e.foundLeader = false
			e.logStore.IncrementTerm()
			e.currentState = CANDIDATE
			log.Info().Msg("starting election")
			e.resetVotes()

			e.electionTimer.Reset(e.conf.ElectionTimeout)

			// channel
			e.broadcastChan <- nil
		}
	}
}

func (e *electionManager) hasRecievedHeartbeat() bool {
	return time.Since(e.heartBeatTime) < e.heartBeatTimeDouble
}

func (e *electionManager) sendVote(lastIndex uint64, lastTerm uint64) bool {
	// if recieved heartbeat already has a leader
	// if grant vote only if the candidate has higher term
	// otherwise the last log entry has the same term, grant vote if candidate has a longer log
	// why have !e.foundLeader? it allows a much easier comparsion to occur
	return !e.foundLeader && e.currentState != LEADER && (lastTerm > e.logStore.GetLatestTerm() ||
		(lastTerm == e.logStore.GetLatestTerm() && lastIndex > e.logStore.GetLatestIndex()))
}
