package raft

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"time"

	"google.golang.org/grpc"
)

type role uint32

const (
	UNKNOWN role = iota
	FOLLOWER
	CANDIDATE
	LEADER
)

type Raft interface {
	Start(net.Listener)
}

type raft struct {
	clients []*raftClient
	grpc    *grpc.Server
	role    role
	id      uint64

	// election
	votes         uint64
	clientedVoted uint64
	electionTimer *time.Timer

	// heart beat
	heartbeatTimer           *time.Timer
	heartBeatTimeout         time.Duration
	heartBeatTimeoutDuration time.Duration
	heartBeatTime            time.Time

	// channels
	voteRequested chan *RequestVotesRequest
	voteReceived  chan *SendVoteRequest
	heartBeatChan chan time.Time

	// indexes/terms
	latestIndex uint64
	latestTerm  uint64
}

func NewRaftServer(servers []Server, logStore LogStore, id uint64) Raft {
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)

	heartBeatChannel := make(chan time.Time, len(servers))
	votesReceivedChan := make(chan *SendVoteRequest, len(servers))
	votesRequestedChan := make(chan *RequestVotesRequest, len(servers))

	grpcServer.RegisterService(&raftService_ServiceDesc, newRaftGrpcServer(logStore, heartBeatChannel, votesRequestedChan, votesReceivedChan))

	clients := []*raftClient{}
	for _, server := range servers {
		client, err := newRaftClient(server.Address, server.Id)
		if err != nil {
			fmt.Println("cannot make client, err=", err)
			continue
		}

		clients = append(clients, client)
	}

	heartBeatTimeout := time.Millisecond * 500

	return &raft{
		clients:                  clients,
		grpc:                     grpcServer,
		role:                     FOLLOWER,
		votes:                    0,
		clientedVoted:            0,
		electionTimer:            time.NewTimer(time.Millisecond * time.Duration(2000+rand.Intn(3000))),
		heartbeatTimer:           time.NewTimer(heartBeatTimeout),
		heartBeatTimeout:         heartBeatTimeout,
		heartBeatTimeoutDuration: 2000 * time.Millisecond,
		heartBeatTime:            time.Unix(0, 0),
		heartBeatChan:            heartBeatChannel,
		latestIndex:              0,
		latestTerm:               0,
		voteReceived:             votesReceivedChan,
		voteRequested:            votesRequestedChan,
		id:                       id,
	}
}

func (r *raft) Start(lis net.Listener) {
	go r.start()

	if err := r.grpc.Serve(lis); err != nil {
		panic(err) // unable to handle this
	}
}

func (r *raft) start() {
	for {
		select {
		case t := <-r.heartBeatChan:
			r.heartBeatTime = t

		case event := <-r.voteReceived:
			// if found another leader
			if r.hasRecievedHeartbeat() || r.role == FOLLOWER {
				fmt.Println("Demoted to follower as found leader")
				r.role = FOLLOWER
				continue
			}

			if event.Voted {
				r.votes++
			}

			r.clientedVoted++

			// if not enough nodes have voted wait
			clientHalf := uint64(len(r.clients)) / 2

			if r.clientedVoted < clientHalf || r.role == LEADER {
				continue
			}

			if r.votes > clientHalf {
				r.role = LEADER
				fmt.Println("I am the leader!")
				go r.startHeartBeats()
			} else {
				r.role = FOLLOWER
				fmt.Println("I am follower :(")
			}

		case event := <-r.voteRequested:
			client, err := r.getClientByID(event.Id)
			if err != nil {
				fmt.Println("couldn't find client who voted", event.Id)
			}

			if _, err := client.gClient.SendVote(context.Background(), &SendVoteRequest{
				Voted: r.sendVote(event.Index, event.Term),
				Id:    r.id,
			}); err != nil {
				fmt.Println("unable to send vote to node, err=", err)
			}

		case <-r.electionTimer.C:
			// only begin election if no heartbeat has started
			if r.hasRecievedHeartbeat() {
				r.electionTimer.Reset(time.Millisecond * time.Duration(rand.Intn(3000)))
				continue
			}

			if r.role == LEADER {
				continue
			}

			r.latestTerm++
			r.role = CANDIDATE

			fmt.Println("Starting/reseting election!")
			r.resetVotes()
			r.broadCastVotes()

			r.electionTimer.Reset(time.Millisecond * time.Duration(3000))
		}
	}
}

func (r *raft) startHeartBeats() {
	hbReq := &HeartBeatRequest{}
	ctx := context.Background()

	for {
		<-r.heartbeatTimer.C

		for _, client := range r.clients {
			ctxTime, cancel := context.WithTimeout(ctx, r.heartBeatTimeoutDuration)
			if _, err := client.gClient.HeartBeat(ctxTime, hbReq); err != nil {
				fmt.Println("Unable to find client:", client.address)
			}
			cancel()
		}

		r.heartbeatTimer.Reset(r.heartBeatTimeout)
	}
}

func (r *raft) broadCastVotes() {
	ctx := context.Background()

	for _, client := range r.clients {
		if _, err := client.gClient.RequestVotes(ctx, &RequestVotesRequest{
			Term:  r.latestTerm,
			Index: r.latestTerm,
			Id:    r.id,
		}); err != nil {
			fmt.Println("Unable to find client:", client.address)
		}
	}
}

func (r *raft) sendVote(lastIndex uint64, lastTerm uint64) bool {
	// if recieved heartbeat already has a leader
	// if grant vote only if the candidate has higher term
	// otherwise the last log entry has the same term, grant vote if candidate has a longer log
	return !r.hasRecievedHeartbeat() && (lastTerm > r.latestTerm || (lastTerm == r.latestTerm && lastIndex >= r.latestIndex))
}

func (r *raft) getClientByID(id uint64) (*raftClient, error) {
	if len(r.clients) == 0 {
		return nil, fmt.Errorf("no clients")
	}

	for _, client := range r.clients {
		if client.id == id {
			return client, nil
		}
	}

	return nil, fmt.Errorf("cannot find client")
}

func (r *raft) hasRecievedHeartbeat() bool {
	return time.Since(r.heartBeatTime) < time.Second*2
}

func (r *raft) resetVotes() {
	r.votes = 1
	r.clientedVoted = 1
}
