package raft

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type raftClient struct {
	gClient raftServiceClient
	conn    *grpc.ClientConn
	address string
	id      uint64
	stream  AppendEntriesStreamClient
}

func newRaftClient(address string, id uint64) (*raftClient, error) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, err
	}

	gClient := newRaftServiceClient(conn)
	stream, err := gClient.AppendEntriesStream(context.Background())
	if err != nil {
		stream = nil
	}

	return &raftClient{
		gClient: gClient,
		conn:    conn,
		address: address,
		id:      id,
		stream:  stream,
	}, nil

}
