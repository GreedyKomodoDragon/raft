package raft

import (
	context "context"
	"fmt"
	"io"
	"time"

	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

type raftGrpcServer interface {
	RequestVotes(context.Context, *RequestVotesRequest) (*RequestVotesResult, error)
	SendVote(context.Context, *SendVoteRequest) (*SendVoteResult, error)
	CommitLog(context.Context, *CommitLogRequest) (*CommitLogResult, error)
	AppendEntriesStream(AppendEntriesStreamServer) error
	PipeEntries(PipeEntriesServer) error
	HeartBeatStream(HeartBeatStreamServer) error
	mustEmbedUnimplementedRaftServiceServer()
}

type raftServer struct {
	logStore     LogStore
	electManager *electionManager

	// heartbeat
	hbResult      *HeartBeatResult
	heartbeatChan chan time.Time

	voteRequested chan *RequestVotesRequest
	voteReceived  chan *SendVoteRequest

	appApply ApplicationApply

	UnimplementedRaftServiceServer
}

func newRaftGrpcServer(logStore LogStore, heartBeatChan chan time.Time, voteRequested chan *RequestVotesRequest,
	voteReceived chan *SendVoteRequest, appApply ApplicationApply, electManager *electionManager) raftGrpcServer {
	return &raftServer{
		logStore:      logStore,
		heartbeatChan: heartBeatChan,
		hbResult:      &HeartBeatResult{},
		voteRequested: voteRequested,
		voteReceived:  voteReceived,
		appApply:      appApply,
		electManager:  electManager,
	}
}

func (r *raftServer) RequestVotes(ctx context.Context, req *RequestVotesRequest) (*RequestVotesResult, error) {
	r.voteRequested <- req
	return &RequestVotesResult{}, nil
}

func (r *raftServer) SendVote(ctx context.Context, req *SendVoteRequest) (*SendVoteResult, error) {
	r.voteReceived <- req

	return &SendVoteResult{}, nil
}

func (r *raftServer) CommitLog(ctx context.Context, req *CommitLogRequest) (*CommitLogResult, error) {
	if !r.electManager.foundLeader {
		fmt.Println("leader not found yet")
		return &CommitLogResult{
			Applied: false,
		}, nil
	}

	log, err := r.logStore.GetLog(req.Index)
	if err != nil {
		return &CommitLogResult{
			Applied: false,
		}, err
	}

	log.LeaderCommited = true

	// return early if piping
	if r.logStore.IsPiping() {
		return &CommitLogResult{
			Applied: true,
		}, nil
	}

	// request the pipeling to begin
	if log.Index-1 > r.logStore.GetLatestIndex() {
		fmt.Println("request piping")
		r.logStore.SetPiping(true)

		return &CommitLogResult{
			Applied: true,
			Missing: r.logStore.GetLatestIndex(),
		}, nil
	}

	if _, err = r.appApply.Apply(Log{
		Index:   req.Index,
		LogType: DATA_LOG,
		Data:    log.Data,
	}); err != nil {
		return &CommitLogResult{
			Applied: false,
		}, err
	}

	r.logStore.IncrementIndex()

	return &CommitLogResult{
		Applied: err == nil,
	}, nil
}

func (r *raftServer) AppendEntriesStream(stream AppendEntriesStreamServer) error {
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

func (r *raftServer) PipeEntries(stream PipeEntriesServer) error {
	first := uint64(0)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			fmt.Println("finished piping")
			return nil
		}

		if err != nil {
			fmt.Println("piping err stream:", err)
			return err
		}

		if err := r.logStore.SetLog(in.Index, &Log{
			Index:          in.Index,
			Data:           in.Data,
			Term:           in.Term,
			LogType:        in.Type,
			LeaderCommited: in.Commited,
		}); err != nil {
			fmt.Println("piping err set:", err)
			return fmt.Errorf("failed to set log")
		}

		if first == 0 {
			first = in.Index
			defer func() {
				fmt.Println("applying from:", first)
				go r.logStore.ApplyFrom(first, r.appApply)
			}()

		}
	}
}

func (r *raftServer) HeartBeatStream(stream HeartBeatStreamServer) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			fmt.Println("heartbeat err stream:", err)
			return err
		}

		r.heartbeatChan <- time.Now()
	}
}

// UnimplementedRaftServiceServer must be embedded to have forward compatible implementations.
type UnimplementedRaftServiceServer struct {
}

func (UnimplementedRaftServiceServer) RequestVotes(context.Context, *RequestVotesRequest) (*RequestVotesResult, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RequestVotes not implemented")
}

func (UnimplementedRaftServiceServer) SendVote(context.Context, *SendVoteRequest) (*SendVoteResult, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SendVote not implemented")
}

func (UnimplementedRaftServiceServer) CommitLog(context.Context, *CommitLogRequest) (*CommitLogResult, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CommitLog not implemented")
}

func (UnimplementedRaftServiceServer) AppendEntriesStream(AppendEntriesStreamServer) error {
	return status.Errorf(codes.Unimplemented, "method AppendEntriesStream not implemented")
}

func (UnimplementedRaftServiceServer) PipeEntries(PipeEntriesServer) error {
	return status.Errorf(codes.Unimplemented, "method PipeEntries not implemented")
}

func (UnimplementedRaftServiceServer) HeartBeatStream(HeartBeatStreamServer) error {
	return status.Errorf(codes.Unimplemented, "method HeartBeatStream not implemented")
}

func (UnimplementedRaftServiceServer) mustEmbedUnimplementedRaftServiceServer() {}

// UnsafeRaftServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to RaftServiceServer will
// result in compilation errors.
type UnsafeRaftServiceServer interface {
	mustEmbedUnimplementedRaftServiceServer()
}

func RegisterRaftServiceServer(s grpc.ServiceRegistrar, srv raftGrpcServer) {
	s.RegisterService(&raftService_ServiceDesc, srv)
}

func _RaftService_RequestVotes_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RequestVotesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(raftGrpcServer).RequestVotes(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/raft.RaftService/RequestVotes",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(raftGrpcServer).RequestVotes(ctx, req.(*RequestVotesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _RaftService_SendVote_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SendVoteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(raftGrpcServer).SendVote(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/raft.RaftService/SendVote",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(raftGrpcServer).SendVote(ctx, req.(*SendVoteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _RaftService_CommitLog_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CommitLogRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(raftGrpcServer).CommitLog(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/raft.RaftService/CommitLog",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(raftGrpcServer).CommitLog(ctx, req.(*CommitLogRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _RaftService_AppendEntriesStream_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(raftGrpcServer).AppendEntriesStream(&raftServiceAppendEntriesStreamServer{stream})
}

type AppendEntriesStreamServer interface {
	Send(*AppendEntriesResult) error
	Recv() (*AppendEntriesRequest, error)
	grpc.ServerStream
}

type raftServiceAppendEntriesStreamServer struct {
	grpc.ServerStream
}

func (x *raftServiceAppendEntriesStreamServer) Send(m *AppendEntriesResult) error {
	return x.ServerStream.SendMsg(m)
}

func (x *raftServiceAppendEntriesStreamServer) Recv() (*AppendEntriesRequest, error) {
	m := new(AppendEntriesRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _RaftService_PipeEntries_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(raftGrpcServer).PipeEntries(&raftServicePipeEntriesServer{stream})
}

type PipeEntriesServer interface {
	Send(*PipeEntriesResponse) error
	Recv() (*PipeEntriesRequest, error)
	grpc.ServerStream
}

type raftServicePipeEntriesServer struct {
	grpc.ServerStream
}

func (x *raftServicePipeEntriesServer) Send(m *PipeEntriesResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *raftServicePipeEntriesServer) Recv() (*PipeEntriesRequest, error) {
	m := new(PipeEntriesRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _RaftService_HeartBeatStream_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(raftGrpcServer).HeartBeatStream(&raftServiceHeartBeatStreamServer{stream})
}

type HeartBeatStreamServer interface {
	Send(*HeartBeatResult) error
	Recv() (*HeartBeatRequest, error)
	grpc.ServerStream
}

type raftServiceHeartBeatStreamServer struct {
	grpc.ServerStream
}

func (x *raftServiceHeartBeatStreamServer) Send(m *HeartBeatResult) error {
	return x.ServerStream.SendMsg(m)
}

func (x *raftServiceHeartBeatStreamServer) Recv() (*HeartBeatRequest, error) {
	m := new(HeartBeatRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// RaftService_ServiceDesc is the grpc.ServiceDesc for RaftService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var raftService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "raft.RaftService",
	HandlerType: (*raftGrpcServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "RequestVotes",
			Handler:    _RaftService_RequestVotes_Handler,
		},
		{
			MethodName: "SendVote",
			Handler:    _RaftService_SendVote_Handler,
		},
		{
			MethodName: "CommitLog",
			Handler:    _RaftService_CommitLog_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "AppendEntriesStream",
			Handler:       _RaftService_AppendEntriesStream_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "PipeEntries",
			Handler:       _RaftService_PipeEntries_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "HeartBeatStream",
			Handler:       _RaftService_HeartBeatStream_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "raft.proto",
}
