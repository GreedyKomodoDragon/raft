package raft

import (
	context "context"

	grpc "google.golang.org/grpc"
)

type raftServiceClient interface {
	RequestVotes(ctx context.Context, in *RequestVotesRequest, opts ...grpc.CallOption) (*RequestVotesResult, error)
	SendVote(ctx context.Context, in *SendVoteRequest, opts ...grpc.CallOption) (*SendVoteResult, error)
	CommitLog(ctx context.Context, in *CommitLogRequest, opts ...grpc.CallOption) (*CommitLogResult, error)
	AppendEntriesStream(ctx context.Context, opts ...grpc.CallOption) (AppendEntriesStreamClient, error)
	PipeEntries(ctx context.Context, opts ...grpc.CallOption) (pipeEntriesClient, error)
	HeartBeatStream(ctx context.Context, opts ...grpc.CallOption) (heartBeatStreamClient, error)
}

type raftGrpcClient struct {
	cc grpc.ClientConnInterface
}

func newRaftServiceClient(cc grpc.ClientConnInterface) raftServiceClient {
	return &raftGrpcClient{cc}
}

func (c *raftGrpcClient) RequestVotes(ctx context.Context, in *RequestVotesRequest, opts ...grpc.CallOption) (*RequestVotesResult, error) {
	out := new(RequestVotesResult)
	err := c.cc.Invoke(ctx, "/raft.RaftService/RequestVotes", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *raftGrpcClient) SendVote(ctx context.Context, in *SendVoteRequest, opts ...grpc.CallOption) (*SendVoteResult, error) {
	out := new(SendVoteResult)
	err := c.cc.Invoke(ctx, "/raft.RaftService/SendVote", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *raftGrpcClient) CommitLog(ctx context.Context, in *CommitLogRequest, opts ...grpc.CallOption) (*CommitLogResult, error) {
	out := new(CommitLogResult)
	err := c.cc.Invoke(ctx, "/raft.RaftService/CommitLog", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *raftGrpcClient) AppendEntriesStream(ctx context.Context, opts ...grpc.CallOption) (AppendEntriesStreamClient, error) {
	stream, err := c.cc.NewStream(ctx, &raftService_ServiceDesc.Streams[0], "/raft.RaftService/AppendEntriesStream", opts...)
	if err != nil {
		return nil, err
	}
	x := &raftServiceAppendEntriesStreamClient{stream}
	return x, nil
}

func (c *raftGrpcClient) PipeEntries(ctx context.Context, opts ...grpc.CallOption) (pipeEntriesClient, error) {
	stream, err := c.cc.NewStream(ctx, &raftService_ServiceDesc.Streams[1], "/raft.RaftService/PipeEntries", opts...)
	if err != nil {
		return nil, err
	}
	x := &raftServicePipeEntriesClient{stream}
	return x, nil
}

type AppendEntriesStreamClient interface {
	Send(*AppendEntriesRequest) error
	Recv() (*AppendEntriesResult, error)
	grpc.ClientStream
}

type raftServiceAppendEntriesStreamClient struct {
	grpc.ClientStream
}

func (x *raftServiceAppendEntriesStreamClient) Send(m *AppendEntriesRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *raftServiceAppendEntriesStreamClient) Recv() (*AppendEntriesResult, error) {
	m := new(AppendEntriesResult)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

type pipeEntriesClient interface {
	Send(*PipeEntriesRequest) error
	Recv() (*PipeEntriesResponse, error)
	grpc.ClientStream
}

type raftServicePipeEntriesClient struct {
	grpc.ClientStream
}

func (x *raftServicePipeEntriesClient) Send(m *PipeEntriesRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *raftServicePipeEntriesClient) Recv() (*PipeEntriesResponse, error) {
	m := new(PipeEntriesResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *raftGrpcClient) HeartBeatStream(ctx context.Context, opts ...grpc.CallOption) (heartBeatStreamClient, error) {
	stream, err := c.cc.NewStream(ctx, &raftService_ServiceDesc.Streams[2], "/raft.RaftService/HeartBeatStream", opts...)
	if err != nil {
		return nil, err
	}
	x := &raftServiceHeartBeatStreamClient{stream}
	return x, nil
}

type heartBeatStreamClient interface {
	Send(*HeartBeatRequest) error
	Recv() (*HeartBeatResult, error)
	grpc.ClientStream
}

type raftServiceHeartBeatStreamClient struct {
	grpc.ClientStream
}

func (x *raftServiceHeartBeatStreamClient) Send(m *HeartBeatRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *raftServiceHeartBeatStreamClient) Recv() (*HeartBeatResult, error) {
	m := new(HeartBeatResult)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}
