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

	voteReceived  chan *SendVoteRequest
	heartBeatTime time.Time
	heartBeatChan chan time.Time

	leaderChanInternal chan interface{}
	broadcastChan      chan interface{}

	clientHalf uint64
}

func newElectionManager(logStore LogStore, voteRequested chan *RequestVotesRequest, voteReceived chan *SendVoteRequest,
	heartBeatChan chan time.Time, clientHalf uint64, leaderChan chan interface{}, leaderChanInternal chan interface{},
	broadcastChan chan interface{}) *electionManager {

	return &electionManager{
		currentState:       FOLLOWER,
		votes:              0,
		clientedVoted:      0,
		logStore:           logStore,
		electionTimer:      time.NewTimer(time.Millisecond * time.Duration(3000+rand.Intn(6000))),
		foundLeader:        false,
		clientHalf:         clientHalf,
		heartBeatTime:      time.Unix(0, 0),
		voteReceived:       voteReceived,
		heartBeatChan:      heartBeatChan,
		leaderChanInternal: leaderChanInternal,
		broadcastChan:      broadcastChan,
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
				e.electionTimer.Reset(time.Millisecond * time.Duration(4000+rand.Intn(2000)))
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

			e.electionTimer.Reset(time.Millisecond * time.Duration(6000))

			// channel
			e.broadcastChan <- nil
		}
	}
}

func (e *electionManager) hasRecievedHeartbeat() bool {
	return time.Since(e.heartBeatTime) < time.Second*4
}

func (e *electionManager) sendVote(lastIndex uint64, lastTerm uint64) bool {
	// if recieved heartbeat already has a leader
	// if grant vote only if the candidate has higher term
	// otherwise the last log entry has the same term, grant vote if candidate has a longer log
	// why have !e.foundLeader? it allows a much easier comparsion to occur
	return !e.foundLeader && e.currentState != LEADER && (lastTerm > e.logStore.GetLatestTerm() ||
		(lastTerm == e.logStore.GetLatestTerm() && lastIndex > e.logStore.GetLatestIndex()))
}
