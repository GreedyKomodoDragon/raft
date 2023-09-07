package raft

import (
	"context"
	"fmt"
	"math/rand"
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

	heartBeatTime time.Time
	heartBeatChan chan time.Time

	// channels
	voteRequested chan *RequestVotesRequest
	voteReceived  chan *SendVoteRequest

	logStore LogStore

	// leader channel
	leaderChan chan interface{}

	// application apply
	appApply ApplicationApply

	// locks
	applyLock  *sync.Mutex
	commitLock *sync.Mutex

	// constants
	reqAppend *AppendEntriesRequest
	reqCommit *CommitLogRequest

	// waitgroup/index counter
	wg    *sync.WaitGroup
	index *AtomicCounter

	// election
	electManager *electionManager
}

func NewRaftServer(servers []Server, logStore LogStore, id uint64) Raft {
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)

	heartBeatChannel := make(chan time.Time, len(servers))
	votesReceivedChan := make(chan *SendVoteRequest, len(servers))
	votesRequestedChan := make(chan *RequestVotesRequest, len(servers))

	appApply := &StdOutApply{}

	if err := logStore.RestoreLogs(appApply); err != nil {
		fmt.Println("unable to read snapshots")
	}

	elect := newElectionManager(logStore)

	grpcServer.RegisterService(&raftService_ServiceDesc, newRaftGrpcServer(logStore, heartBeatChannel, votesRequestedChan, votesReceivedChan, appApply, elect))

	clients := []*raftClient{}
	for _, server := range servers {
		client, err := newRaftClient(server.Address, server.Id)
		if err != nil {
			fmt.Println("cannot make client, err=", err)
			continue
		}

		clients = append(clients, client)
	}

	return &raft{
		clients:       clients,
		grpc:          grpcServer,
		logStore:      logStore,
		heartBeatTime: time.Unix(0, 0),
		heartBeatChan: heartBeatChannel,
		voteReceived:  votesReceivedChan,
		voteRequested: votesRequestedChan,
		id:            id,
		leaderChan:    make(chan interface{}, 1),
		appApply:      appApply,
		applyLock:     &sync.Mutex{},
		commitLock:    &sync.Mutex{},
		clientHalf:    uint64(len(clients))/2 + 1,
		reqAppend:     &AppendEntriesRequest{},
		reqCommit:     &CommitLogRequest{},
		wg:            nil,
		index:         nil,
		electManager:  elect,
	}
}

func (r *raft) LeaderChan() chan interface{} {
	return r.leaderChan
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
			r.electManager.foundLeader = true

			if r.electManager.currentState == CANDIDATE {
				fmt.Println("Demoted to follower as found leader")
				r.electManager.currentState = FOLLOWER

			}

		case event := <-r.voteReceived:
			if r.electManager.currentState == LEADER {
				fmt.Println("already leader so ignored")
				continue
			}

			// if found another leader
			if r.hasRecievedHeartbeat() || r.electManager.currentState == FOLLOWER {
				fmt.Println("Demoted to follower as found leader")
				r.electManager.currentState = FOLLOWER
				continue
			}

			if event.Voted {
				fmt.Println("vote found:", event.Id)
				r.electManager.votes++
			}

			r.electManager.clientedVoted++

			if r.electManager.clientedVoted < r.clientHalf {
				fmt.Println("not enough votes found")
				continue
			}

			if r.electManager.votes < r.clientHalf {
				fmt.Println("not enough votes in favour")
				r.electManager.currentState = FOLLOWER
				continue
			}

			fmt.Println("results:", r.electManager.votes, r.clientHalf)

			r.electManager.currentState = LEADER
			fmt.Println("became leader")

			r.buildStreams()
			r.startStream()

			// send logs to followers and self
			if _, err := r.ApplyLog([]byte{}, RAFT_LOG); err != nil {
				fmt.Println("unable to append leader log", err)
				continue
			}

			// run in goroutine to avoid stopping if ignored
			go func() { r.leaderChan <- nil }()

		case event := <-r.voteRequested:
			client, err := r.getClientByID(event.Id)
			if err != nil {
				fmt.Println("couldn't find client who voted", event.Id)
			}

			fmt.Println("voted:", r.sendVote(event.Index, event.Term))
			if _, err := client.gClient.SendVote(context.Background(), &SendVoteRequest{
				Voted: r.sendVote(event.Index, event.Term),
				Id:    r.id,
			}); err != nil {
				fmt.Println("unable to send vote to node, err=", err)
			}

		case <-r.electManager.electionTimer.C:
			// only begin election if no heartbeat has started
			if r.hasRecievedHeartbeat() {
				r.electManager.electionTimer.Reset(time.Millisecond * time.Duration(4000+rand.Intn(2000)))
				continue
			}

			if r.electManager.currentState == LEADER {
				continue
			}

			r.electManager.foundLeader = false
			r.logStore.IncrementTerm()
			r.electManager.currentState = CANDIDATE

			fmt.Println("Starting/reseting election!")
			r.electManager.resetVotes()
			r.broadCastVotes()

			r.electManager.electionTimer.Reset(time.Millisecond * time.Duration(6000))
		}
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

func (r *raft) sendVote(lastIndex uint64, lastTerm uint64) bool {
	// if recieved heartbeat already has a leader
	// if grant vote only if the candidate has higher term
	// otherwise the last log entry has the same term, grant vote if candidate has a longer log
	// why have !r.electManager.foundLeader? it allows a much easier comparsion to occur
	return !r.electManager.foundLeader && r.electManager.currentState != LEADER && (lastTerm > r.logStore.GetLatestTerm() ||
		(lastTerm == r.logStore.GetLatestTerm() && lastIndex > r.logStore.GetLatestIndex()))
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
	return time.Since(r.heartBeatTime) < time.Second*4
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

	r.wg = wg
	r.index = counter

	for _, client := range r.clients {
		if client.stream == nil {
			wg.Done()
			continue
		}

		go r.appendClient(ctx, client, &AppendEntriesRequest{
			Index: latestIndex,
			Term:  r.logStore.GetLatestTerm(),
			Type:  DATA_LOG,
			Data:  data,
		}, wg, counter)
	}

	if err := r.logStore.AppendLog(&reqLog); err != nil {
		fmt.Println("leader append err:", err)
		return nil, err
	}

	wg.Wait()
	if uint64(counter.IdCount()) < r.clientHalf-1 {
		fmt.Println("failed to confirm log, count is:", counter.IdCount(), r.clientHalf-1)
		return nil, fmt.Errorf("failed to confirm log")
	}

	r.logStore.IncrementIndex()
	reqLog.LeaderCommited = true
	r.commitLock.Lock()

	return &reqLog, nil

}

func (r *raft) appendClient(ctx context.Context, client *raftClient, req *AppendEntriesRequest, wg *sync.WaitGroup, atom *AtomicCounter) {
	if err := client.stream.Send(req); err != nil {
		// fmt.Println("failed to send:", err)
		wg.Done()
		return
	}

	// timeout
	go func() {
		time.Sleep(3 * time.Second)
		if atom.HasId(client.id) {
			return
		}

		atom.AddIdOnly(client.id)
		wg.Done()
	}()
}

func (r *raft) applyClient(ctx context.Context, client *raftClient, commitReq *CommitLogRequest, wg *sync.WaitGroup) {
	defer wg.Done()

	result, err := client.gClient.CommitLog(ctx, commitReq)
	if err != nil {
		// fmt.Println("client append err:", err)
		return
	}

	if result.Missing != 0 && !client.piping {
		// start piping logs
		fmt.Println("needs to start piping now")
		go r.startPipingStream(client, result.Missing, commitReq.Index)
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
		go func(client *raftClient) {
		START:
			for {
				if client.stream == nil {
					break
				}

				in, err := client.stream.Recv()
				if err != nil {
					break
				}

				if r.index.HasId(client.id) {
					fmt.Println("already has id")
					continue
				}

				if !in.Applied {
					fmt.Println("not applied")
					r.wg.Done()
					continue
				}

				r.index.Increment(client.id)
				r.wg.Done()
			}

			for {
				stream, err := client.gClient.AppendEntriesStream(context.Background())
				if err != nil {
					time.Sleep(2 * time.Second)
					fmt.Println("cannot find client, err:", err)
				}

				client.stream = stream
				goto START

			}
		}(client)

		go func(client *raftClient) {
			defer fmt.Println("exited the loop")
		START:
			for {
				if client.heartBeatStream == nil {
					break
				}

				if err := client.heartBeatStream.Send(&HeartBeatRequest{}); err != nil {
					fmt.Println("failed to send heart:", client.id)
					break
				}

				client.heartbeatTimer.Reset(client.heartDur)
				<-client.heartbeatTimer.C
			}

			for {
				stream, err := client.gClient.HeartBeatStream(context.Background())
				if err != nil {
					time.Sleep(2 * time.Second)
					fmt.Println("cannot find client", client.id, " err:", err)
				}

				client.heartBeatStream = stream
				goto START

			}
		}(client)

	}
}

func (r *raft) startPipingStream(client *raftClient, startIndex, endIndex uint64) error {
	// reset once finished
	defer func() {
		client.pipestream = nil
		client.piping = false
	}()

	current := startIndex

	for i := 0; client.pipestream == nil && i < 3; i++ {
		stream, err := client.gClient.PipeEntries(context.Background())
		if err == nil {
			client.pipestream = stream
			break
		}

		if i == 2 {
			return err
		}

		time.Sleep(time.Second)
		fmt.Println("cannot find client, err:", err)
	}

	for current <= endIndex {
		log, err := r.logStore.GetLog(current)
		if err != nil {
			fmt.Println("cannot find log:", current, err)
			client.pipestream.CloseSend()
			return err
		}

		if err := client.pipestream.Send(&PipeEntriesRequest{
			Index:    current,
			Data:     log.Data,
			Commited: log.LeaderCommited,
			Term:     log.Term,
			Type:     log.LogType,
		}); err != nil {
			fmt.Println("err:", err)
			return err
		}

		current++
	}

	client.pipestream.CloseSend()
	return nil
}
