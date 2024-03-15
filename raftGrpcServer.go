package raft

import (
	context "context"
	"io"
	"time"

	"github.com/rs/zerolog/log"
)

type raftServer struct {
	logStore      LogStore
	electManager  *electionManager
	hbResult      *HeartBeatResult
	voteRequested chan *RequestVotesRequest
	appApply      ApplicationApply

	UnimplementedRaftGRPCServer
}

func newRaftGrpcServer(logStore LogStore, voteRequested chan *RequestVotesRequest, appApply ApplicationApply, electManager *electionManager) RaftGRPCServer {
	return &raftServer{
		logStore:      logStore,
		hbResult:      &HeartBeatResult{},
		voteRequested: voteRequested,
		appApply:      appApply,
		electManager:  electManager,
	}
}

func (r *raftServer) RequestVotes(ctx context.Context, req *RequestVotesRequest) (*RequestVotesResult, error) {
	r.voteRequested <- req
	return &RequestVotesResult{}, nil
}

func (r *raftServer) SendVote(ctx context.Context, req *SendVoteRequest) (*SendVoteResult, error) {
	r.electManager.voteReceived <- req

	return &SendVoteResult{}, nil
}

func (r *raftServer) CommitLog(stream RaftGRPC_CommitLogServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		lg, err := r.logStore.GetLog(req.Index)
		if err != nil {
			if err := stream.Send(&CommitLogResult{
				Applied: false,
				Missing: 0,
			}); err != nil {
				return err
			}

			continue
		}

		lg.LeaderCommited = true

		// return early if piping
		if r.logStore.IsPiping() {
			if err := stream.Send(&CommitLogResult{
				Applied: true,
			}); err != nil {
				return err
			}

			continue
		}

		// request the pipeling to begin
		if lg.Index-1 > r.logStore.GetLatestIndex() {
			log.Info().Msg("requesting piping")
			r.logStore.SetPiping(true)

			if err := stream.Send(&CommitLogResult{
				Applied: true,
				Missing: r.logStore.GetLatestIndex() + 1,
			}); err != nil {
				return err
			}

			continue
		}

		if _, err = r.appApply.Apply(Log{
			Index:   req.Index,
			LogType: DATA_LOG,
			Data:    lg.Data,
		}); err != nil {
			if err := stream.Send(&CommitLogResult{
				Applied: false,
			}); err != nil {
				return err
			}

			continue
		}

		r.logStore.IncrementIndex()

		if err := stream.Send(&CommitLogResult{
			Applied: true,
		}); err != nil {
			return err
		}
	}
}

func (r *raftServer) AppendEntriesStream(stream RaftGRPC_AppendEntriesStreamServer) error {
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		err = r.logStore.AppendLog(&Log{
			Term:    in.Term,
			Index:   in.Index,
			LogType: in.Type,
			Data:    in.Data,
		})

		if err := stream.Send(&AppendEntriesResult{
			Applied: err == nil,
			Index:   in.Index,
		}); err != nil {
			return err
		}

	}
}

func (r *raftServer) PipeEntries(stream RaftGRPC_PipeEntriesServer) error {
	first := uint64(0)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			log.Info().Msg("finished piping")
			return nil
		}

		if err != nil {
			log.Err(err).Msg("issue while piping logs")
			return err
		}

		if err := r.logStore.SetLog(in.Index, &Log{
			Index:          in.Index,
			Data:           in.Data,
			Term:           in.Term,
			LogType:        in.Type,
			LeaderCommited: in.Commited,
		}); err != nil {
			log.Err(err).Msg("issue when setting log")
			return err
		}

		if first == 0 {
			first = in.Index
			defer func() {
				go r.logStore.ApplyFrom(first, r.appApply)
			}()

		}
	}
}

func (r *raftServer) HeartBeatStream(stream RaftGRPC_HeartBeatStreamServer) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			log.Err(err).Msg("issue while receiving heartbeat")
			return err
		}

		r.electManager.heartBeatChan <- time.Now()
	}
}
