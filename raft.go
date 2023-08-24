package raft

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strings"
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
	role    role
	id      uint64

	// constants
	clientHalf uint64

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
	applyedLogs   chan *Result

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
	wgMap      map[uint64]*sync.WaitGroup
	indexCount map[uint64]*AtomicCounter
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

	grpcServer.RegisterService(&raftService_ServiceDesc, newRaftGrpcServer(logStore, heartBeatChannel, votesRequestedChan, votesReceivedChan, appApply))

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
	tOut := 500 + rand.Intn(6000)

	return &raft{
		clients:                  clients,
		grpc:                     grpcServer,
		role:                     FOLLOWER,
		votes:                    0,
		clientedVoted:            0,
		logStore:                 logStore,
		electionTimer:            time.NewTimer(time.Millisecond * time.Duration(tOut)),
		heartbeatTimer:           time.NewTimer(heartBeatTimeout),
		heartBeatTimeout:         heartBeatTimeout,
		heartBeatTimeoutDuration: 2000 * time.Millisecond,
		heartBeatTime:            time.Unix(0, 0),
		heartBeatChan:            heartBeatChannel,
		voteReceived:             votesReceivedChan,
		voteRequested:            votesRequestedChan,
		id:                       id,
		leaderChan:               make(chan interface{}, 1),
		applyedLogs:              make(chan *Result, 5),
		appApply:                 appApply,
		applyLock:                &sync.Mutex{},
		commitLock:               &sync.Mutex{},
		clientHalf:               uint64(len(clients)) / 2,
		reqAppend:                &AppendEntriesRequest{},
		reqCommit:                &CommitLogRequest{},
		wgMap:                    make(map[uint64]*sync.WaitGroup),
		indexCount:               make(map[uint64]*AtomicCounter),
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

			if r.role == CANDIDATE {
				fmt.Println("Demoted to follower as found leader")
				r.role = FOLLOWER
			}

		case event := <-r.voteReceived:
			if r.role == LEADER {
				fmt.Println("already leader so ignored")
				continue
			}

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

			if r.clientedVoted < clientHalf {
				fmt.Println("not enough votes found")
				continue
			}

			if r.votes < clientHalf {
				fmt.Println("not enough votes in favour")
				r.role = FOLLOWER
				continue
			}

			r.role = LEADER
			go r.startHeartBeats()
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

			r.logStore.IncrementTerm()
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
	return !r.hasRecievedHeartbeat() &&
		(lastTerm > r.logStore.GetLatestTerm() ||
			(lastTerm == r.logStore.GetLatestTerm() && lastIndex >= r.logStore.GetLatestIndex()))
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

func (r *raft) ApplyLog(data []byte, typ uint64) ([]byte, error) {
	latestIndex, err := r.broadCastAppendLog(data, typ)
	if err != nil {
		return nil, err
	}

	defer r.commitLock.Unlock()
	if typ == RAFT_LOG {
		return nil, nil
	}

	var wg sync.WaitGroup
	r.reqCommit.Index = latestIndex

	ctx := context.Background()

	wg.Add(2)
	for _, client := range r.clients {
		go r.applyClient(ctx, client, r.reqCommit, &wg)
	}

	// apply log
	data, err = r.appApply.Apply(Log{
		Index:   latestIndex,
		Data:    data,
		LogType: typ,
	})

	if err != nil {
		fmt.Println("failed to apply log:", err)
	}

	wg.Wait()
	return data, nil
}

func (r *raft) broadCastAppendLog(data []byte, typ uint64) (uint64, error) {
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
	wg.Add(2)
	counter := NewAtomicCounter()

	r.applyLock.Lock()
	defer r.applyLock.Unlock()

	latestIndex := r.logStore.GetLatestIndex()
	r.reqAppend.Index = latestIndex
	reqLog.Index = latestIndex

	r.wgMap[latestIndex] = wg

	r.indexCount[latestIndex] = counter

	for _, client := range r.clients {
		go r.appendClient(ctx, client, &AppendEntriesRequest{
			Index: latestIndex,
			Term:  r.logStore.GetLatestTerm(),
			Type:  DATA_LOG,
			Data:  data,
		}, wg, counter)
	}

	if err := r.logStore.AppendLog(reqLog); err != nil {
		fmt.Println("leader append err:", err)
		return 0, err
	}

	wg.Wait()
	if uint64(counter.IdCount()) <= r.clientHalf-1 {
		fmt.Println("failed to confirm log, count is:", counter.IdCount())
		return 0, fmt.Errorf("failed to confirm log")
	}

	r.logStore.IncrementIndex()
	r.commitLock.Lock()

	return latestIndex, nil

}

func (r *raft) appendClient(ctx context.Context, client *raftClient, req *AppendEntriesRequest, wg *sync.WaitGroup, atom *AtomicCounter) {
	if client.stream == nil {
		fmt.Println("no stream found")
		wg.Done()
		return
	}

	if err := client.stream.Send(req); err != nil {
		fmt.Println("failed to send")
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
		fmt.Println("client append err:", err)
		return
	}

	if !result.Applied {
		fmt.Println("client failed to append")
	}
}

func (r *raft) buildStreams() {
	for _, client := range r.clients {
		for i := 0; i < 3; i++ {
			stream, err := client.gClient.AppendEntriesStream(context.Background())
			if err != nil {
				time.Sleep(2 * time.Second)
				fmt.Println("cannot find client, err:", err)
				continue
			}

			client.stream = stream
		}
	}
}

func (r *raft) startStream() {
	for _, client := range r.clients {
		go func(client *raftClient) {
			defer fmt.Println("exited the loop")
		START:
			for {
				if client.stream == nil {
					break
				}

				in, err := client.stream.Recv()
				if err != nil {
					if strings.Contains(err.Error(), "error reading from server: EOF") {
						fmt.Println("recv: err eof")
						break
					}

					fmt.Println("recv: err continue", err)
					continue
				}

				counter, ok := r.indexCount[in.Index]
				if !ok {
					fmt.Println("not found counter")
					continue
				}

				if counter.HasId(client.id) {
					fmt.Println("already has id")
					continue
				}

				wg, ok := r.wgMap[in.Index]
				if !ok {
					fmt.Println("not found wg")
					continue
				}

				if !in.Applied {
					fmt.Println("not applied")
					wg.Done()
					continue
				}

				counter.Increment(client.id)
				wg.Done()
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

	}
}
