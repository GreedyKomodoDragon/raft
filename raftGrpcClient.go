package raft

import (
	context "context"

	grpc "google.golang.org/grpc"
)

type raftServiceClient interface {
	GetStatus(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResult, error)
	RequestVotes(ctx context.Context, in *RequestVotesRequest, opts ...grpc.CallOption) (*RequestVotesResult, error)
	AppendEntries(ctx context.Context, in *AppendEntriesRequest, opts ...grpc.CallOption) (*AppendEntriesResult, error)
	HeartBeat(ctx context.Context, in *HeartBeatRequest, opts ...grpc.CallOption) (*HeartBeatResult, error)
	SendVote(ctx context.Context, in *SendVoteRequest, opts ...grpc.CallOption) (*SendVoteResult, error)
	CommitLog(ctx context.Context, in *CommitLogRequest, opts ...grpc.CallOption) (*CommitLogResult, error)
}

type raftGrpcClient struct {
	cc grpc.ClientConnInterface
}

func newRaftServiceClient(cc grpc.ClientConnInterface) raftServiceClient {
	return &raftGrpcClient{cc}
}

func (c *raftGrpcClient) GetStatus(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResult, error) {
	out := new(StatusResult)
	err := c.cc.Invoke(ctx, "/raft.RaftService/GetStatus", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *raftGrpcClient) RequestVotes(ctx context.Context, in *RequestVotesRequest, opts ...grpc.CallOption) (*RequestVotesResult, error) {
	out := new(RequestVotesResult)
	err := c.cc.Invoke(ctx, "/raft.RaftService/RequestVotes", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *raftGrpcClient) AppendEntries(ctx context.Context, in *AppendEntriesRequest, opts ...grpc.CallOption) (*AppendEntriesResult, error) {
	out := new(AppendEntriesResult)
	err := c.cc.Invoke(ctx, "/raft.RaftService/AppendEntries", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *raftGrpcClient) HeartBeat(ctx context.Context, in *HeartBeatRequest, opts ...grpc.CallOption) (*HeartBeatResult, error) {
	out := new(HeartBeatResult)
	err := c.cc.Invoke(ctx, "/raft.RaftService/HeartBeat", in, out, opts...)
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
