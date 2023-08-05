package raft

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
)

type Raft interface {
	Start(net.Listener)
}

type raft struct {
	clients []*raftClient
	grpc    *grpc.Server
}

func NewRaftServer(servers []Server, logStore LogStore) Raft {
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)

	grpcServer.RegisterService(&raftService_ServiceDesc, newRaftGrpcServer(logStore))

	clients := []*raftClient{}
	for _, server := range servers {
		client, err := newRaftClient(server.Address)
		if err != nil {
			fmt.Println(err)
			continue
		}

		clients = append(clients, client)
	}

	return &raft{
		clients: clients,
		grpc:    grpcServer,
	}
}

func (r *raft) Start(lis net.Listener) {
	go func() {
		time.Sleep(500 * time.Millisecond)

		for i := uint64(0); true; i++ {
			fmt.Println("i", i)
			for _, client := range r.clients {
				_, err := client.gClient.AppendEntries(context.Background(), &AppendEntriesRequest{
					Term: i,
					Type: []byte{},
					Data: []byte{},
				})

				fmt.Println(err)
			}
		}

	}()
	if err := r.grpc.Serve(lis); err != nil {
		panic(err) // unable to handle this
	}

}
