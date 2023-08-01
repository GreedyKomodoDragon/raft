package raft

import (
	context "context"

	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

type raftGrpcServer interface {
	GetStatus(context.Context, *StatusRequest) (*StatusResult, error)
	RequestVotes(context.Context, *RequestVotesRequest) (*RequestVotesResult, error)
	AppendEntries(AppendEntriesServer) error
	HeartBeat(HeartBeatServer) error
	mustEmbedUnimplementedRaftServiceServer()
}

type raftServer struct {
	UnimplementedRaftServiceServer
}

func newRaftGrpcServer() raftGrpcServer {
	return &raftServer{}
}

func (r *raftServer) GetStatus(ctx context.Context, req *StatusRequest) (*StatusResult, error) {
	return nil, nil
}

func (r *raftServer) RequestVotes(ctx context.Context, req *RequestVotesRequest) (*RequestVotesResult, error) {
	return nil, nil
}

func (r *raftServer) AppendEntries(server AppendEntriesServer) error {
	return nil
}

func (r *raftServer) HeartBeat(server HeartBeatServer) error {
	return nil
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
func (UnimplementedRaftServiceServer) AppendEntries(AppendEntriesServer) error {
	return status.Errorf(codes.Unimplemented, "method AppendEntries not implemented")
}
func (UnimplementedRaftServiceServer) HeartBeat(HeartBeatServer) error {
	return status.Errorf(codes.Unimplemented, "method HeartBeat not implemented")
}
func (UnimplementedRaftServiceServer) mustEmbedUnimplementedRaftServiceServer() {}

// UnsafeRaftServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to RaftServiceServer will
// result in compilation errors.
type UnsafeRaftServiceServer interface {
	mustEmbedUnimplementedRaftServiceServer()
}

func RegisterRaftServiceServer(s grpc.ServiceRegistrar, srv raftGrpcServer) {
	s.RegisterService(&RaftService_ServiceDesc, srv)
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

func _RaftService_AppendEntries_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(raftGrpcServer).AppendEntries(&raftServiceAppendEntriesServer{stream})
}

type AppendEntriesServer interface {
	SendAndClose(*AppendEntriesResult) error
	Recv() (*AppendEntriesRequest, error)
	grpc.ServerStream
}

type raftServiceAppendEntriesServer struct {
	grpc.ServerStream
}

func (x *raftServiceAppendEntriesServer) SendAndClose(m *AppendEntriesResult) error {
	return x.ServerStream.SendMsg(m)
}

func (x *raftServiceAppendEntriesServer) Recv() (*AppendEntriesRequest, error) {
	m := new(AppendEntriesRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _RaftService_HeartBeat_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(raftGrpcServer).HeartBeat(&raftServiceHeartBeatServer{stream})
}

type HeartBeatServer interface {
	SendAndClose(*HeartBeatResult) error
	Recv() (*HeartBeatRequest, error)
	grpc.ServerStream
}

type raftServiceHeartBeatServer struct {
	grpc.ServerStream
}

func (x *raftServiceHeartBeatServer) SendAndClose(m *HeartBeatResult) error {
	return x.ServerStream.SendMsg(m)
}

func (x *raftServiceHeartBeatServer) Recv() (*HeartBeatRequest, error) {
	m := new(HeartBeatRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// RaftService_ServiceDesc is the grpc.ServiceDesc for RaftService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var RaftService_ServiceDesc = grpc.ServiceDesc{
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
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "AppendEntries",
			Handler:       _RaftService_AppendEntries_Handler,
			ClientStreams: true,
		},
		{
			StreamName:    "HeartBeat",
			Handler:       _RaftService_HeartBeat_Handler,
			ClientStreams: true,
		},
	},
	Metadata: "raft.proto",
}
