package raft

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

type chanItem struct {
	atom *AtomicCounter
	req  *AppendEntriesRequest
}

type raftClient struct {
	gClient         raftServiceClient
	conn            *grpc.ClientConn
	address         string
	id              uint64
	stream          AppendEntriesStreamClient
	pipestream      pipeEntriesClient
	heartBeatStream heartBeatStreamClient
	piping          bool
	heartDur        time.Duration
	atom            *AtomicCounter
	wg              *sync.WaitGroup
	conf            *ClientConfig
	appendChannel   chan *chanItem
}

func newRaftClient(server Server, heartBeatDur time.Duration, conf *ClientConfig, wg *sync.WaitGroup) (*raftClient, error) {
	conn, err := grpc.Dial(server.Address, server.Opts...)
	if err != nil {
		return nil, err
	}
	return &raftClient{
		gClient:       newRaftServiceClient(conn),
		conn:          conn,
		address:       server.Address,
		id:            server.Id,
		piping:        false,
		heartDur:      heartBeatDur,
		conf:          conf,
		wg:            wg,
		appendChannel: make(chan *chanItem, 1),
	}, nil
}

func (r *raftClient) buildAppendStream() error {
	ctx := context.Background()
	for i := 0; i < r.conf.StreamBuildAttempts; i++ {
		stream, err := r.gClient.AppendEntriesStream(ctx)
		if err != nil {
			time.Sleep(r.conf.StreamBuildTimeout)
			continue
		}

		r.stream = stream
		return nil
	}

	return fmt.Errorf("failed to create append stream")
}

func (r *raftClient) buildHeartbeatStream() error {
	ctx := context.Background()
	for i := 0; i < r.conf.StreamBuildAttempts; i++ {
		stream, err := r.gClient.HeartBeatStream(ctx)
		if err != nil {
			time.Sleep(r.conf.StreamBuildTimeout)
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
	ctx := context.Background()
	for i := 0; r.pipestream == nil && i < r.conf.StreamBuildAttempts; i++ {
		stream, err := r.gClient.PipeEntries(ctx)
		if err == nil {
			r.pipestream = stream
			break
		}

		if i == r.conf.StreamBuildAttempts-1 {
			return err
		}

		time.Sleep(r.conf.StreamBuildTimeout)
	}

	for current <= endIndex {
		lg, err := logStore.GetLog(current)
		if err != nil {
			log.Error().Uint64("index", current).Err(err).Msg("cannot find log in pipe")
			r.pipestream.CloseSend()
			return err
		}

		if err := r.pipestream.Send(&PipeEntriesRequest{
			Index:    current,
			Data:     lg.Data,
			Commited: lg.LeaderCommited,
			Term:     lg.Term,
			Type:     lg.LogType,
		}); err != nil {
			log.Error().Uint64("index", current).Err(err).Msg("failed to send")
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
				log.Error().Uint64("clientId", r.id).Err(err).Msg("failed to send heart")
				break
			}

			time.Sleep(r.heartDur)
		}

		for {
			stream, err := r.gClient.HeartBeatStream(context.Background())
			if err != nil {
				log.Error().Uint64("clientId", r.id).Err(err).Msg("failed to find client in heartbeat stream")
				time.Sleep(r.heartDur)
				continue
			}

			r.heartBeatStream = stream
			break
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
				log.Debug().Uint64("clientId", r.id).Err(err).Msg("already has id in result stream")
				continue
			}

			if !in.Applied {
				log.Debug().Uint64("clientId", r.id).Err(err).Msg("not applied in result stream")
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
			log.Error().Uint64("clientId", r.id).Err(err).Msg("cannot find client for append stream")
			time.Sleep(r.conf.StreamBuildTimeout)
			continue
		}

		r.stream = stream
		break
	}
}

func (r *raftClient) append() {
	for item := range r.appendChannel {
		r.atom = item.atom

		if err := r.stream.Send(item.req); err != nil {
			log.Error().Uint64("clientId", r.id).Err(err).Msg("failed to send in append stream")
			r.wg.Done()
			return
		}

		// start timeout
		go r.timeout(item.atom)
	}
}

func (r *raftClient) timeout(atom *AtomicCounter) {
	time.Sleep(r.conf.AppendTimeout)
	if atom.HasId(r.id) {
		return
	}

	atom.AddIdOnly(r.id)
	r.wg.Done()
}
