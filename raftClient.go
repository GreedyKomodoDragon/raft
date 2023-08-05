package raft

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type raftClient struct {
	gClient raftServiceClient
	conn    *grpc.ClientConn
	address string
}

func newRaftClient(address string) (*raftClient, error) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, err
	}

	return &raftClient{
		gClient: newRaftServiceClient(conn),
		conn:    conn,
		address: address,
	}, nil

}
