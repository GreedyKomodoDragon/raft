package raft

import (
	context "context"
	"fmt"

	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

type raftGrpcServer interface {
	GetStatus(context.Context, *StatusRequest) (*StatusResult, error)
	RequestVotes(context.Context, *RequestVotesRequest) (*RequestVotesResult, error)
	AppendEntries(context.Context, *AppendEntriesRequest) (*AppendEntriesResult, error)
	HeartBeat(context.Context, *HeartBeatRequest) (*HeartBeatResult, error)
	mustEmbedUnimplementedRaftServiceServer()
}

type raftServer struct {
	logStore LogStore
	UnimplementedRaftServiceServer
}

func newRaftGrpcServer(logStore LogStore) raftGrpcServer {
	return &raftServer{
		logStore: logStore,
	}
}

func (r *raftServer) GetStatus(ctx context.Context, req *StatusRequest) (*StatusResult, error) {
	return nil, nil
}

func (r *raftServer) RequestVotes(ctx context.Context, req *RequestVotesRequest) (*RequestVotesResult, error) {
	return nil, nil
}

func (r *raftServer) AppendEntries(ctx context.Context, req *AppendEntriesRequest) (*AppendEntriesResult, error) {
	r.logStore.AppendLog(Log{
		Term:    req.Term,
		LogType: req.Type,
		Data:    req.Data,
	})

	fmt.Println(req.Term)
	return &AppendEntriesResult{Applied: true}, nil
}

func (r *raftServer) HeartBeat(context.Context, *HeartBeatRequest) (*HeartBeatResult, error) {
	return nil, nil
}

// UnimplementedRaftServiceServer must be embedded to have forward compatible implementations.
type UnimplementedRaftServiceServer struct {
}

func (UnimplementedRaftServiceServer) GetStatus(context.Context, *StatusRequest) (*StatusResult, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetStatus not implemented")
}
func (UnimplementedRaftServiceServer) RequestVotes(context.Context, *RequestVotesRequest) (*RequestVotesResult, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RequestVotes not implemented")
}
func (UnimplementedRaftServiceServer) AppendEntries(context.Context, *AppendEntriesRequest) (*AppendEntriesResult, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AppendEntries not implemented")
}
func (UnimplementedRaftServiceServer) HeartBeat(context.Context, *HeartBeatRequest) (*HeartBeatResult, error) {
	return nil, status.Errorf(codes.Unimplemented, "method HeartBeat not implemented")
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

func _RaftService_GetStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(raftGrpcServer).GetStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/raft.RaftService/GetStatus",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(raftGrpcServer).GetStatus(ctx, req.(*StatusRequest))
	}
	return interceptor(ctx, in, info, handler)
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

func _RaftService_AppendEntries_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AppendEntriesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(raftGrpcServer).AppendEntries(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/raft.RaftService/AppendEntries",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(raftGrpcServer).AppendEntries(ctx, req.(*AppendEntriesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _RaftService_HeartBeat_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HeartBeatRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(raftGrpcServer).HeartBeat(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/raft.RaftService/HeartBeat",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(raftGrpcServer).HeartBeat(ctx, req.(*HeartBeatRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// raftService_ServiceDesc is the grpc.ServiceDesc for RaftService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var raftService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "raft.RaftService",
	HandlerType: (*raftGrpcServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetStatus",
			Handler:    _RaftService_GetStatus_Handler,
		},
		{
			MethodName: "RequestVotes",
			Handler:    _RaftService_RequestVotes_Handler,
		},
		{
			MethodName: "AppendEntries",
			Handler:    _RaftService_AppendEntries_Handler,
		},
		{
			MethodName: "HeartBeat",
			Handler:    _RaftService_HeartBeat_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "raft.proto",
}
