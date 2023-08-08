package raft

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type raftClient struct {
	gClient raftServiceClient
	conn    *grpc.ClientConn
	address string
	id      uint64
}

func newRaftClient(address string, id uint64) (*raftClient, error) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, err
	}

	return &raftClient{
		gClient: newRaftServiceClient(conn),
		conn:    conn,
		address: address,
		id:      id,
	}, nil

}
