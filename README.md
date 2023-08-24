# Raft Consenus
A implementation of the Raft consenus algorithm, written in [Go](https://go.dev/) and utilising GRPC & protobuf to communicate between nodes to obtain consenus. 

## Building

```bash
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    raft.proto
```

## Features for v0.1.0
List of features for tracking progress
### Elections/Following
- [ ] Elections able to pick leader on start up
- [ ] Able to re-select leader for leader falls
- [ ] Node able to re-join leader
- [ ] Log sent on election

### Logs
- [ ] Able to pipe logs if missing
- [ ] Provide interface for custom applications
- [ ] Incremental Snapshots Log Store Storage
- [ ] Incremental Snapshots reloading on start-up
- [ ] Can fetch logs from disk if not found in memory

### Tidying up/Tech Debt
- [ ] Environment out the log store values
- [ ] Environment out the election timeouts
- [ ] Profile & find bottlenecks

## Features for v0.2.0
List of features for tracking progress

### Logs
- [ ] Can create different streams from main stream
    - Benefit is that it would allow different log streams to be on the same nodes, means concurrent logs can be confirmed at the same time if they do not need to interact with each other
