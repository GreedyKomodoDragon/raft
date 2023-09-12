package raft

import (
	"context"
	"fmt"
	"sync"
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
	atom            *AtomicCounter
	wg              *sync.WaitGroup
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
		gClient:        gClient,
		conn:           conn,
		address:        address,
		id:             id,
		piping:         false,
		heartbeatTimer: time.NewTimer(heartDur),
		heartDur:       heartDur,
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

func (r *raftClient) startPiping(logStore LogStore, startIndex, endIndex uint64) error {
	// reset once finished
	defer func() {
		r.pipestream = nil
		r.piping = false
	}()

	current := startIndex

	for i := 0; r.pipestream == nil && i < 3; i++ {
		stream, err := r.gClient.PipeEntries(context.Background())
		if err == nil {
			r.pipestream = stream
			break
		}

		if i == 2 {
			return err
		}

		time.Sleep(time.Second)
		fmt.Println("cannot find client, err:", err)
	}

	for current <= endIndex {
		log, err := logStore.GetLog(current)
		if err != nil {
			fmt.Println("cannot find log:", current, err)
			r.pipestream.CloseSend()
			return err
		}

		if err := r.pipestream.Send(&PipeEntriesRequest{
			Index:    current,
			Data:     log.Data,
			Commited: log.LeaderCommited,
			Term:     log.Term,
			Type:     log.LogType,
		}); err != nil {
			fmt.Println("err:", err)
			return err
		}

		current++
	}

	r.pipestream.CloseSend()
	return nil
}

func (r *raftClient) startheartBeat() {
	hb := &HeartBeatRequest{}
	for {
		for {
			if r.heartBeatStream == nil {
				break
			}

			if err := r.heartBeatStream.Send(hb); err != nil {
				fmt.Println("failed to send heart:", r.id)
				break
			}

			r.heartbeatTimer.Reset(r.heartDur)
			<-r.heartbeatTimer.C
		}

		for {
			stream, err := r.gClient.HeartBeatStream(context.Background())
			if err != nil {
				time.Sleep(2 * time.Second)
				fmt.Println("cannot find client", r.id, " err:", err)
			}

			r.heartBeatStream = stream
		}
	}
}

func (r *raftClient) startApplyResultStream() {
	for {
		for {
			if r.stream == nil {
				break
			}

			in, err := r.stream.Recv()
			if err != nil {
				break
			}

			if r.atom.HasId(r.id) {
				fmt.Println("already has id")
				continue
			}

			if !in.Applied {
				fmt.Println("not applied")
				r.wg.Done()
				continue
			}

			r.atom.Increment(r.id)
			r.wg.Done()
		}

		r.recreateAppendStream()
	}

}

// will loop until stream is established
func (r *raftClient) recreateAppendStream() {
	for {
		stream, err := r.gClient.AppendEntriesStream(context.Background())
		if err != nil {
			time.Sleep(2 * time.Second)
			fmt.Println("cannot find client, err:", err)
		}

		r.stream = stream
	}
}

func (r *raftClient) append(ctx context.Context, req *AppendEntriesRequest, wg *sync.WaitGroup, atom *AtomicCounter) {
	r.atom = atom
	r.wg = wg

	if err := r.stream.Send(req); err != nil {
		r.wg.Done()
		return
	}

	// start timeout
	go r.timeout(wg, atom)
}

func (r *raftClient) timeout(wg *sync.WaitGroup, atom *AtomicCounter) {
	time.Sleep(3 * time.Second)
	if atom.HasId(r.id) {
		return
	}

	atom.AddIdOnly(r.id)
	wg.Done()
}
