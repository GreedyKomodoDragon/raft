package raft

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

type Role uint32

const (
	UNKNOWN Role = iota
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
	Start()
	ApplyLog([]byte, uint64) (interface{}, error)
	LeaderChan() chan interface{}
	State() Role
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
	reqCommit *CommitLogRequest

	// election
	electManager *electionManager

	// raft configurations
	conf *RaftConfig
}

func NewRaftServer(app ApplicationApply, logStore LogStore, grpcServer *grpc.Server, conf *Configuration) Raft {
	votesRequestedChan := make(chan *RequestVotesRequest, len(conf.RaftConfig.Servers))

	if err := logStore.RestoreLogs(app); err != nil {
		log.Info().Msg("Unable to read in snapshots successfully")
	}

	clients := []*raftClient{}
	for _, server := range conf.RaftConfig.Servers {
		client, err := newRaftClient(server, conf.ElectionConfig.HeartbeatTimeout, conf.RaftConfig.ClientConf)
		if err != nil {
			log.Error().Err(err).Msg("Unable to create client")
			continue
		}

		clients = append(clients, client)
	}

	elect := newElectionManager(logStore, len(conf.RaftConfig.Servers), conf.ElectionConfig)
	grpcServer.RegisterService(&raftService_ServiceDesc, newRaftGrpcServer(logStore, votesRequestedChan, app, elect))

	return &raft{
		clients:       clients,
		grpc:          grpcServer,
		logStore:      logStore,
		id:            conf.RaftConfig.Id,
		appApply:      app,
		applyLock:     &sync.Mutex{},
		commitLock:    &sync.Mutex{},
		clientHalf:    uint64(len(clients)/2 + 1),
		reqCommit:     &CommitLogRequest{},
		electManager:  elect,
		voteRequested: votesRequestedChan,
		leaderChan:    make(chan interface{}, 1),
		conf:          conf.RaftConfig,
	}
}

func (r *raft) LeaderChan() chan interface{} {
	return r.leaderChan
}

func (r *raft) Start() {
	go r.electManager.start()
	go r.start()
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
			log.Error().Err(err).Str("address", client.address).Msg("Unable to find client")
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
				log.Error().Err(err).Msg("unable to send leader log")
			}

			// now streams are established
			go func() { r.leaderChan <- nil }()

		case <-r.electManager.broadcastChan:
			r.broadCastVotes()

		case event := <-r.voteRequested:
			client, err := r.getClientByID(event.Id)
			if err != nil {
				log.Error().Err(err).Uint64("clientId", event.Id).Msg("couldn't find client who voted")
			}

			if _, err := client.gClient.SendVote(context.Background(), &SendVoteRequest{
				Voted: r.electManager.sendVote(event.Index, event.Term),
				Id:    r.id,
			}); err != nil {
				log.Error().Err(err).Uint64("clientId", event.Id).Msg("unable to send vote to node")
			}
		}
	}

}

func (r *raft) ApplyLog(data []byte, typ uint64) (interface{}, error) {
	lg, err := r.broadCastAppendLog(data, typ)
	if err != nil {
		return nil, err
	}

	// TODO: fix this messy code up
	defer r.commitLock.Unlock()

	if typ == RAFT_LOG {
		return nil, nil
	}

	var wg sync.WaitGroup
	r.reqCommit.Index = lg.Index

	ctx := context.Background()

	wg.Add(len(r.clients))
	for _, client := range r.clients {
		go r.applyClient(ctx, client, r.reqCommit, &wg)
	}

	// apply log
	dat, err := r.appApply.Apply(*lg)
	if err != nil {
		log.Error().Err(err).Msg("failed to apply log")
	}

	wg.Wait()
	return dat, nil
}

func (r *raft) broadCastAppendLog(data []byte, typ uint64) (*Log, error) {
	ctx := context.Background()

	// TODO: replace with reset-able value
	reqAppend := &AppendEntriesRequest{
		Term: r.logStore.GetLatestTerm(),
		Type: typ,
		Data: data,
	}

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
	reqAppend.Index = latestIndex
	reqLog.Index = latestIndex

	for _, client := range r.clients {
		if client.stream == nil {
			wg.Done()
			continue
		}

		go client.append(ctx, reqAppend, wg, counter)
	}

	if err := r.logStore.AppendLog(&reqLog); err != nil {
		log.Error().Err(err).Msg("failed to append as leader")
		return nil, err
	}

	wg.Wait()
	limit := r.clientHalf - 1
	if uint64(counter.IdCount()) < limit {
		log.Error().Int("counter", counter.IdCount()).Uint64("limit", limit).Msg("failed to confirm log")
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
		log.Error().Err(err).Msg("client failed to append")
		return
	}

	if result.Missing != 0 && !client.piping {
		// start piping logs
		log.Info().Uint64("clientId", client.id).Msg("start piping to client")
		go client.startPiping(r.logStore, result.Missing, commitReq.Index)
	}

	if !result.Applied {
		log.Error().Uint64("clientId", client.id).Uint64("index", commitReq.Index).Msg("client failed to commit")
	}
}

func (r *raft) buildStreams() {
	wg := &sync.WaitGroup{}
	wg.Add(len(r.clients) * 2)

	for _, client := range r.clients {
		go func(client *raftClient) {
			defer wg.Done()
			if err := client.buildAppendStream(); err != nil {
				log.Error().Err(err).Msg("unable to build append stream")
			}
		}(client)

		go func(client *raftClient) {
			defer wg.Done()
			if err := client.buildHeartbeatStream(); err != nil {
				log.Error().Err(err).Msg("unable to build heartbeat stream")
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

func (r *raft) State() Role {
	return r.electManager.currentState
}
