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
	clients             []*raftClient
	grpc                *grpc.Server
	role                role
	electionTimer       *time.Timer
	heartbeatTimer      *time.Timer
	heartBeatTimeout    time.Duration
	heartBeatReqTimeout time.Duration
}

func NewRaftServer(servers []Server, logStore LogStore) Raft {
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)

	grpcServer.RegisterService(&raftService_ServiceDesc, newRaftGrpcServer(logStore))

	clients := []*raftClient{}
	for _, server := range servers {
		client, err := newRaftClient(server.Address)
		if err != nil {
			fmt.Println(err)
			continue
		}

		clients = append(clients, client)
	}

	heartBeatTimeout := time.Millisecond * 500

	return &raft{
		clients:             clients,
		grpc:                grpcServer,
		role:                FOLLOWER,
		electionTimer:       time.NewTimer(time.Millisecond * time.Duration(rand.Intn(500))),
		heartbeatTimer:      time.NewTimer(heartBeatTimeout),
		heartBeatTimeout:    heartBeatTimeout,
		heartBeatReqTimeout: 100 * time.Millisecond,
	}
}

func (r *raft) Start(lis net.Listener) {
	go r.start()

	if err := r.grpc.Serve(lis); err != nil {
		panic(err) // unable to handle this
	}
}

func (r *raft) start() {
	time.Sleep(1000 * time.Millisecond)
	go r.startHeartBeats()

	for {
		select {
		// case event := <-r.voteCastedEvent:
		// 	if !event.Voted {
		// 		// check if
		// 		return
		// 	}

		case <-r.electionTimer.C:
			// r.becomeCandidate()
			// r.broadCastVotes()
		}
	}
}

func (r *raft) startHeartBeats() {
	hbReq := &HeartBeatRequest{}
	ctx := context.Background()

	for {
		<-r.heartbeatTimer.C

		for _, client := range r.clients {
			ctxTime, cancel := context.WithTimeout(ctx, r.heartBeatReqTimeout)
			if _, err := client.gClient.HeartBeat(ctxTime, hbReq); err != nil {
				fmt.Println("Unable to find client:", client.address)
			}
			cancel()
		}

		r.heartbeatTimer.Reset(r.heartBeatTimeout)
	}
}
