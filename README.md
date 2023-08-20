# raft
A implementation of the Raft consenus algorithm, written in [Go](https://go.dev/) and utilising GRPC & protobuf to communicate between nodes to obtain consenus. 

## Building

```bash
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    raft.proto
```