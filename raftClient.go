package raft

import (
	"context"
	"fmt"
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

func (r *raftClient) buildAppendStream() error {
	for i := 0; i < 3; i++ {
		stream, err := r.gClient.AppendEntriesStream(context.Background())
		if err != nil {
			time.Sleep(2 * time.Second)
			fmt.Println("cannot find client, err:", err)
			continue
		}

		r.stream = stream
		return nil
	}

	return fmt.Errorf("failed to create append stream")
}

func (r *raftClient) buildHeartbeatStream() error {
	for i := 0; i < 3; i++ {
		stream, err := r.gClient.HeartBeatStream(context.Background())
		if err != nil {
			time.Sleep(2 * time.Second)
			fmt.Println("cannot find client, err:", err)
			continue
		}

		r.heartBeatStream = stream
		return nil
	}

	return fmt.Errorf("failed to create heartbeat stream")
}
