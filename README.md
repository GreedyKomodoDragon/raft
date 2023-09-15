# Raft Consenus
A implementation of the [Raft consenus algorithm](https://raft.github.io/), written in [Go](https://go.dev/) and utilising GRPC & protobuf to communicate between nodes to obtain consenus.

## Current State
The raft library is currently in a very alpha stage, and does not have all the features that would make the library more use-able in production, such as resolving election issues using non-acting nodes.

### Raft-Algorithm Features
List of features for tracking progress
* Elections:
    * Elections able to pick leader on start up
    * Able to re-select leader after leader failures
    * Node able to re-join

* Raft Logs
    * Able to pipe logs if missing
    * Provide interface for custom applications
    * Incremental Snapshots Log Store Storage
    * Incremental Snapshots reloading on start-up
    * Can fetch logs from disk if not found in memory


### Additional Library Provided Features

Most of these features can be replaced with your own, e.g. if you want to write/read to S3 for you snapshots you can write an inject your own Logstore to handle this.

#### Configuration

The library has a high-level of configuration out of the box. There is a configuration struct to set every setting to your required setting, even down to how each node should communication using grpc.

If you want you can write a completely new LogStore and it should work within the raft system.

#### Incremental Snapshots

This library supports application level increment snapshots, using our provide LogStore. This ensures there is no big spike in computation when writing out your logs to disk, compared to a full snapshot of the logs.

##### Current Caveats 
Few things to note when using our current snapshot system:

* You must have enough storage to hold all the logs for either how long you are running the system, or expect downtime if you have to move from one volume to another.
* Each Node currently outputs its own snapshot, they do not push to a central place/a single node performs this task.
* When reading in a sector of the snapshot, it will read in the whole sector into memory, dependent on your memory usage this may kill the node
* The location of where it writes to is fixed, this will not change for local disk written

## Testing

Currently the testing is performed manually. This is performed by spinning up a docker-compose cluster and then getting the nodes to fire they're own logs to the other nodes.

Once the API has been confirmed and is much more stable, this is when we will begin written unit tests to ensure confidence in the solution.