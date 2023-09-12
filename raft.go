package raft

import (
	"context"
	"fmt"
	"net"
	"sync"
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

type Result struct {
	Index uint64
	Data  []byte
	Err   error
}

type Raft interface {
	Start(net.Listener)
	ApplyLog([]byte, uint64) ([]byte, error)
	LeaderChan() chan interface{}
}

type raft struct {
	clients []*raftClient
	grpc    *grpc.Server
	id      uint64

	// constants
	clientHalf uint64

	logStore LogStore

	voteRequested chan *RequestVotesRequest
	leaderChan    chan interface{}

	// application apply
	appApply ApplicationApply

	// locks
	applyLock  *sync.Mutex
	commitLock *sync.Mutex

	// constants
	reqAppend *AppendEntriesRequest
	reqCommit *CommitLogRequest

	// election
	electManager *electionManager
}

func NewRaftServer(servers []Server, logStore LogStore, id uint64) Raft {
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)

	heartBeatChannel := make(chan time.Time, len(servers))
	votesReceivedChan := make(chan *SendVoteRequest, len(servers))
	votesRequestedChan := make(chan *RequestVotesRequest, len(servers))
	leaderChan := make(chan interface{}, 1)
	leaderChanInternal := make(chan interface{}, 1)
	broadcastChan := make(chan interface{}, 1)

	appApply := &StdOutApply{}

	if err := logStore.RestoreLogs(appApply); err != nil {
		fmt.Println("unable to read snapshots")
	}

	clients := []*raftClient{}
	for _, server := range servers {
		client, err := newRaftClient(server.Address, server.Id)
		if err != nil {
			fmt.Println("cannot make client, err=", err)
			continue
		}

		clients = append(clients, client)
	}

	elect := newElectionManager(logStore, votesRequestedChan, votesReceivedChan, heartBeatChannel,
		uint64(len(clients))/2+1, leaderChan, leaderChanInternal, broadcastChan)

	grpcServer.RegisterService(&raftService_ServiceDesc, newRaftGrpcServer(logStore, heartBeatChannel, votesRequestedChan, votesReceivedChan, appApply, elect))

	return &raft{
		clients:       clients,
		grpc:          grpcServer,
		logStore:      logStore,
		id:            id,
		appApply:      appApply,
		applyLock:     &sync.Mutex{},
		commitLock:    &sync.Mutex{},
		clientHalf:    uint64(len(clients))/2 + 1,
		reqAppend:     &AppendEntriesRequest{},
		reqCommit:     &CommitLogRequest{},
		electManager:  elect,
		voteRequested: votesRequestedChan,
		leaderChan:    leaderChan,
	}
}

func (r *raft) LeaderChan() chan interface{} {
	return r.leaderChan
}

func (r *raft) Start(lis net.Listener) {
	go r.electManager.start()
	go r.start()

	if err := r.grpc.Serve(lis); err != nil {
		panic(err) // unable to handle this
	}
}

func (r *raft) broadCastVotes() {
	ctx := context.Background()
	req := &RequestVotesRequest{
		Term:  r.logStore.GetLatestTerm(),
		Index: r.logStore.GetLatestIndex(),
		Id:    r.id,
	}

	for _, client := range r.clients {
		if _, err := client.gClient.RequestVotes(ctx, req); err != nil {
			fmt.Println("Unable to find client:", client.address)
		}
	}
}

func (r *raft) start() {
	for {
		select {
		case <-r.electManager.leaderChanInternal:
			r.buildStreams()
			r.startStream()

			// send logs to followers and self
			if _, err := r.ApplyLog([]byte{}, RAFT_LOG); err != nil {
				fmt.Println("unable to append leader log", err)
			}

			// now streams are established
			go func() { r.leaderChan <- nil }()

		case <-r.electManager.broadcastChan:
			r.broadCastVotes()

		case event := <-r.voteRequested:
			client, err := r.getClientByID(event.Id)
			if err != nil {
				fmt.Println("couldn't find client who voted", event.Id)
			}

			if _, err := client.gClient.SendVote(context.Background(), &SendVoteRequest{
				Voted: r.electManager.sendVote(event.Index, event.Term),
				Id:    r.id,
			}); err != nil {
				fmt.Println("unable to send vote to node, err=", err)
			}
		}
	}

}

func (r *raft) ApplyLog(data []byte, typ uint64) ([]byte, error) {
	log, err := r.broadCastAppendLog(data, typ)
	if err != nil {
		return nil, err
	}

	// TODO: fix this messy code up
	defer r.commitLock.Unlock()

	if typ == RAFT_LOG {
		return nil, nil
	}

	var wg sync.WaitGroup
	r.reqCommit.Index = log.Index

	ctx := context.Background()

	wg.Add(2)
	for _, client := range r.clients {
		go r.applyClient(ctx, client, r.reqCommit, &wg)
	}

	// apply log
	data, err = r.appApply.Apply(*log)
	if err != nil {
		fmt.Println("failed to apply log:", err)
	}

	wg.Wait()
	return data, nil
}

func (r *raft) broadCastAppendLog(data []byte, typ uint64) (*Log, error) {
	ctx := context.Background()

	r.reqAppend.Term = r.logStore.GetLatestTerm()
	r.reqAppend.Type = typ
	r.reqAppend.Data = data

	reqLog := Log{
		Term:           r.logStore.GetLatestTerm(),
		LogType:        DATA_LOG,
		Data:           data,
		LeaderCommited: false,
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(r.clients))
	counter := NewAtomicCounter()

	r.applyLock.Lock()
	defer r.applyLock.Unlock()

	latestIndex := r.logStore.GetLatestIndex()
	r.reqAppend.Index = latestIndex
	reqLog.Index = latestIndex

	for _, client := range r.clients {
		if client.stream == nil {
			wg.Done()
			continue
		}

		go client.append(ctx, r.reqAppend, wg, counter)
	}

	if err := r.logStore.AppendLog(&reqLog); err != nil {
		fmt.Println("leader append err:", err)
		return nil, err
	}

	wg.Wait()
	limit := r.clientHalf - 1
	if uint64(counter.IdCount()) < limit {
		fmt.Println("failed to append log, count is:", counter.IdCount(), limit, latestIndex)
		return nil, fmt.Errorf("failed to confirm log")
	}

	r.logStore.IncrementIndex()
	reqLog.LeaderCommited = true
	r.commitLock.Lock()

	return &reqLog, nil

}

func (r *raft) applyClient(ctx context.Context, client *raftClient, commitReq *CommitLogRequest, wg *sync.WaitGroup) {
	defer wg.Done()

	result, err := client.gClient.CommitLog(ctx, commitReq)
	if err != nil {
		fmt.Println("client append err:", err)
		return
	}

	if result.Missing != 0 && !client.piping {
		// start piping logs
		fmt.Println("needs to start piping now")
		go client.startPiping(r.logStore, result.Missing, commitReq.Index)
	}

	if !result.Applied {
		fmt.Println("client failed to commit:", commitReq.Index, client.id)
	}
}

func (r *raft) buildStreams() {
	wg := &sync.WaitGroup{}
	wg.Add(len(r.clients) * 2)

	for _, client := range r.clients {
		go func(client *raftClient) {
			defer wg.Done()
			if err := client.buildAppendStream(); err != nil {
				fmt.Println("err:", err)
			}
		}(client)

		go func(client *raftClient) {
			defer wg.Done()
			if err := client.buildHeartbeatStream(); err != nil {
				fmt.Println("err:", err)
			}
		}(client)
	}

	wg.Wait()
}

func (r *raft) startStream() {
	for _, client := range r.clients {
		go client.startApplyResultStream()
		go client.startheartBeat()
	}
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
