package raft

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type raftClient struct {
	gClient    raftServiceClient
	conn       *grpc.ClientConn
	address    string
	id         uint64
	stream     AppendEntriesStreamClient
	pipestream pipeEntriesClient
	piping     bool
}

func newRaftClient(address string, id uint64) (*raftClient, error) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, err
	}

	gClient := newRaftServiceClient(conn)

	return &raftClient{
		gClient:    gClient,
		conn:       conn,
		address:    address,
		id:         id,
		stream:     nil,
		pipestream: nil,
		piping:     false,
	}, nil

}
