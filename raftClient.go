package raft

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type raftClient struct {
	gClient         raftServiceClient
	conn            *grpc.ClientConn
	address         string
	id              uint64
	stream          AppendEntriesStreamClient
	pipestream      pipeEntriesClient
	heartBeatStream heartBeatStreamClient
	piping          bool
	heartbeatTimer  *time.Timer
	heartDur        time.Duration
}

func newRaftClient(address string, id uint64) (*raftClient, error) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, err
	}

	gClient := newRaftServiceClient(conn)

	heartDur := time.Second * 2

	return &raftClient{
		gClient:         gClient,
		conn:            conn,
		address:         address,
		id:              id,
		stream:          nil,
		pipestream:      nil,
		heartBeatStream: nil,
		piping:          false,
		heartbeatTimer:  time.NewTimer(heartDur),
		heartDur:        heartDur,
	}, nil

}
