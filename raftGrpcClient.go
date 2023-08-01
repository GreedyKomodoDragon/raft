package raft

import (
	context "context"

	grpc "google.golang.org/grpc"
)

type raftServiceClient interface {
	GetStatus(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResult, error)
	RequestVotes(ctx context.Context, in *RequestVotesRequest, opts ...grpc.CallOption) (*RequestVotesResult, error)
	AppendEntries(ctx context.Context, opts ...grpc.CallOption) (raftService_AppendEntriesClient, error)
	HeartBeat(ctx context.Context, opts ...grpc.CallOption) (raftService_HeartBeatClient, error)
}

type raftClient struct {
	cc grpc.ClientConnInterface
}

func newRaftServiceClient(cc grpc.ClientConnInterface) raftServiceClient {
	return &raftClient{cc}
}

func (c *raftClient) GetStatus(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResult, error) {
	out := new(StatusResult)
	err := c.cc.Invoke(ctx, "/raft.RaftService/GetStatus", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *raftClient) RequestVotes(ctx context.Context, in *RequestVotesRequest, opts ...grpc.CallOption) (*RequestVotesResult, error) {
	out := new(RequestVotesResult)
	err := c.cc.Invoke(ctx, "/raft.RaftService/RequestVotes", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *raftClient) AppendEntries(ctx context.Context, opts ...grpc.CallOption) (raftService_AppendEntriesClient, error) {
	stream, err := c.cc.NewStream(ctx, &RaftService_ServiceDesc.Streams[0], "/raft.RaftService/AppendEntries", opts...)
	if err != nil {
		return nil, err
	}
	x := &raftServiceAppendEntriesClient{stream}
	return x, nil
}

type raftService_AppendEntriesClient interface {
	Send(*AppendEntriesRequest) error
	CloseAndRecv() (*AppendEntriesResult, error)
	grpc.ClientStream
}

type raftServiceAppendEntriesClient struct {
	grpc.ClientStream
}

func (x *raftServiceAppendEntriesClient) Send(m *AppendEntriesRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *raftServiceAppendEntriesClient) CloseAndRecv() (*AppendEntriesResult, error) {
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	m := new(AppendEntriesResult)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *raftClient) HeartBeat(ctx context.Context, opts ...grpc.CallOption) (raftService_HeartBeatClient, error) {
	stream, err := c.cc.NewStream(ctx, &RaftService_ServiceDesc.Streams[1], "/raft.RaftService/HeartBeat", opts...)
	if err != nil {
		return nil, err
	}
	x := &raftServiceHeartBeatClient{stream}
	return x, nil
}

type raftService_HeartBeatClient interface {
	Send(*HeartBeatRequest) error
	CloseAndRecv() (*HeartBeatResult, error)
	grpc.ClientStream
}

type raftServiceHeartBeatClient struct {
	grpc.ClientStream
}

func (x *raftServiceHeartBeatClient) Send(m *HeartBeatRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *raftServiceHeartBeatClient) CloseAndRecv() (*HeartBeatResult, error) {
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	m := new(HeartBeatResult)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}
