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
	Start(grpc *grpc.Server)
	ApplyLog(*[]byte, uint64) (interface{}, error)
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

	commitWait *sync.WaitGroup
	appendWait *sync.WaitGroup
}

func NewRaftServer(app ApplicationApply, logStore LogStore, conf *Configuration) Raft {
	if err := logStore.RestoreLogs(app); err != nil {
		log.Error().Err(err).Msg("Unable to read in snapshots successfully")
	} else {
		log.Info().Uint64("index", logStore.GetLatestIndex()).Msg("restore id")
	}

	appendWait := &sync.WaitGroup{}
	commitWait := &sync.WaitGroup{}

	clients := []*raftClient{}
	for _, server := range conf.RaftConfig.Servers {
		client, err := newRaftClient(server, conf.ElectionConfig.HeartbeatTimeout, conf.RaftConfig.ClientConf, appendWait, commitWait, logStore)
		if err != nil {
			log.Error().Err(err).Msg("Unable to create client")
			continue
		}

		clients = append(clients, client)
	}

	elect := newElectionManager(logStore, len(conf.RaftConfig.Servers), conf.ElectionConfig)

	return &raft{
		clients:      clients,
		logStore:     logStore,
		id:           conf.RaftConfig.Id,
		appApply:     app,
		applyLock:    &sync.Mutex{},
		commitLock:   &sync.Mutex{},
		clientHalf:   uint64(len(clients)/2 + 1),
		reqCommit:    &CommitLogRequest{},
		electManager: elect,
		leaderChan:   make(chan interface{}, 1),
		conf:         conf.RaftConfig,
		commitWait:   commitWait,
		appendWait:   appendWait,
	}
}

func (r *raft) LeaderChan() chan interface{} {
	return r.leaderChan
}

func (r *raft) Start(grpcServer *grpc.Server) {
	votesRequestedChan := make(chan *RequestVotesRequest, len(r.conf.Servers))

	// register on start
	grpcServer.RegisterService(&raftGRPC_ServiceDesc, newRaftGrpcServer(r.logStore, votesRequestedChan, r.appApply, r.electManager))

	r.voteRequested = votesRequestedChan
	r.grpc = grpcServer

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
			if _, err := r.ApplyLog(&[]byte{}, RAFT_LOG); err != nil {
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

func (r *raft) ApplyLog(data *[]byte, typ uint64) (interface{}, error) {
	lg, err := r.broadCastAppendLog(data, typ)
	if err != nil {
		return nil, err
	}

	// TODO: fix this messy code up
	defer r.commitLock.Unlock()

	if typ == RAFT_LOG {
		return nil, nil
	}

	r.reqCommit.Index = lg.Index

	item := &commitItem{
		atom: NewAtomicCounter(),
		req:  r.reqCommit,
	}

	r.commitWait.Add(len(r.clients))
	for _, client := range r.clients {
		if client.commitStream == nil {
			r.commitWait.Done()
			continue
		}

		client.commitChan <- item
	}

	// apply log
	dat, err := r.appApply.Apply(*lg)
	if err != nil {
		log.Error().Err(err).Msg("failed to apply log")
	}

	r.commitWait.Wait()
	return dat, err
}

func (r *raft) broadCastAppendLog(data *[]byte, typ uint64) (*Log, error) {
	ctx := context.Background()
	defer ctx.Done()

	// TODO: replace with reset-able value
	reqAppend := &AppendEntriesRequest{
		Term: r.logStore.GetLatestTerm(),
		Type: typ,
		Data: *data,
	}

	// TODO: re-use
	reqLog := Log{
		Term:           r.logStore.GetLatestTerm(),
		LogType:        DATA_LOG,
		Data:           *data,
		LeaderCommited: false,
	}

	r.applyLock.Lock()
	defer r.applyLock.Unlock()

	r.appendWait.Add(len(r.clients))

	latestIndex := r.logStore.GetLatestIndex()
	reqAppend.Index = latestIndex
	reqLog.Index = latestIndex

	// TODO: re-use
	chanItem := &chanItem{
		atom: NewAtomicCounter(),
		req:  reqAppend,
	}

	for _, client := range r.clients {
		if client.stream == nil {
			r.appendWait.Done()
			continue
		}

		client.appendChannel <- chanItem
	}

	if err := r.logStore.AppendLog(&reqLog); err != nil {
		log.Error().Err(err).Msg("failed to append as leader")
		return nil, err
	}

	r.appendWait.Wait()

	// TODO: re-use
	limit := r.clientHalf - 1

	if uint64(chanItem.atom.IdCount()) < limit {
		log.Error().Int("counter", chanItem.atom.IdCount()).Uint64("limit", limit).Msg("failed to append log")
		return nil, fmt.Errorf("failed to append log")
	}

	r.logStore.IncrementIndex()
	reqLog.LeaderCommited = true
	r.commitLock.Lock()

	return &reqLog, nil
}

func (r *raft) buildStreams() {
	wg := &sync.WaitGroup{}
	wg.Add(len(r.clients) * 3)

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

		go func(client *raftClient) {
			defer wg.Done()
			if err := client.buildCommitStream(); err != nil {
				log.Error().Err(err).Msg("unable to build commit stream")
			}
		}(client)
	}

	wg.Wait()
}

func (r *raft) startStream() {
	for _, client := range r.clients {
		go client.startAppendResultStream()
		go client.append()
		go client.startheartBeat()
		go client.startCommitStream()
		go client.commitSend()
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
