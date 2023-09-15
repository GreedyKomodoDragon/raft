package raft

import (
	"time"

	"google.golang.org/grpc"
)

type Configuration struct {
	RaftConfig     *RaftConfig
	ElectionConfig *ElectionConfig
}

type RaftConfig struct {
	Id         uint64
	Servers    []Server
	ServerOpts []grpc.ServerOption
	ClientConf *ClientConfig
}

type ClientConfig struct {
	StreamBuildTimeout  time.Duration
	StreamBuildAttempts int
	AppendTimeout       time.Duration
}

type Server struct {
	Address string
	Id      uint64
	Opts    []grpc.DialOption
}

type ElectionConfig struct {
	ElectionTimeout  time.Duration
	HeartbeatTimeout time.Duration
}
