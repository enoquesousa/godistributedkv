<div align="center">
  <h1>Go Distributed KV</h1>
  <p>A distributed, modular, and resilient Key-Value store using gRPC for communication and consensus algorithms for data replication.</p>

  <img src="assets/github-go.png" alt="Go Distributed KV Banner" width="600px">

  <br>

  ![CI](https://github.com/enoquesousa/godistributedkv/actions/workflows/go.yml/badge.svg)
  [![CodeFactor](https://www.codefactor.io/repository/github/enoquesousa/godistributedkv/badge)](https://www.codefactor.io/repository/github/enoquesousa/godistributedkv)
  ![Go Reference](https://pkg.go.dev/badge/github.com/enoquesousa/godistributedkv.svg)
  ![License](https://img.shields.io/github/license/enoquesousa/godistributedkv)
  ![Go Version](https://img.shields.io/github/go-mod/go-version/enoquesousa/godistributedkv)
  ![Last Commit](https://img.shields.io/github/last-commit/enoquesousa/godistributedkv)
</div>

---

**godistributedkv** is a distributed storage engine designed for high availability and consistency. It implements quorum-based replication (majority), automatic leader election, and persistence via Write-Ahead Log (WAL), ensuring data is safely replicated across a cluster of nodes.

## Development Roadmap

- [x] **Phase 1: The Core (In-Memory KV Store)**
  - **Objective:** Create the basic data structure and ensure thread safety.
  - **What was done:** Implementation of a struct encapsulating a `map[string]string` with `sync.RWMutex`, providing `Get`, `Set`, and `Delete` methods, alongside a basic gRPC server.

- [x] **Phase 2: Clustering (Discovery and Networking)**
  - **Objective:** Enable multiple nodes to discover and communicate with each other via gRPC.
  - **What was done:** Peer list management via configuration, inter-node gRPC communication, and health checks using `Ping`.

- [x] **Phase 3: Simplified Consensus (Leader and Election)**
  - **Objective:** Determine which node coordinates the cluster to avoid write conflicts.
  - **What was done:** Implementation of a Raft-inspired election algorithm (Terms, Votes, Heartbeats). Only the leader accepts `Set` operations, with follower redirection.

- [x] **Phase 4: Log Replication**
  - **Objective:** Ensure all followers replicate the data saved by the leader.
  - **What was done:** Replication of `Set` operations via gRPC with quorum confirmation (N/2 + 1) before local application and client response.

- [x] **Phase 5: Persistence and Recovery (WAL)**
  - **Objective:** Ensure data durability in case of failure or restart.
  - **What was done:** Implementation of a persistent Write-Ahead Log (WAL). Automatic state reconstruction at boot and orchestration via Docker Compose for fault tolerance testing.

## Quick Start

### Prerequisites

- **Go 1.24+**
- **Protocol Buffers Compiler (protoc)**
- **gRPC Plugins for Go**

### Installation

```bash
git clone https://github.com/enoquesousa/godistributedkv.git
cd godistributedkv
make tidy
```

### Running (Multi-Node)

To start a local cluster with 3 nodes, open different terminals and run:

```bash
# Node 1 (Suggested initial leader)
go run cmd/server/main.go --addr :50051 --peers :50052,:50053

# Node 2
go run cmd/server/main.go --addr :50052 --peers :50051,:50053

# Node 3
go run cmd/server/main.go --addr :50053 --peers :50051,:50052
```

## Makefile Targets

| Target         | Description                               |
| -------------- | ----------------------------------------- |
| `make build`   | Compiles the server binary to `bin/`      |
| `make run`     | Runs the server directly via Go           |
| `make test`    | Executes the unit test suite              |
| `make tidy`    | Cleans and updates `go.mod` dependencies  |
| `make protoc`  | Generates gRPC files from `.proto`        |
| `make clean`   | Removes binaries and temporary logs       |

## Architecture

The project follows **Dependency Inversion** principles and a **Modular Architecture**, separating the core storage from the networking infrastructure.

- `api/proto`: gRPC contract definitions and data services.
- `cmd/server`: Application entry point and cluster bootstrap.
- `internal/cluster`: Consensus orchestrator, leader election, and node health.
- `internal/config`: Typed configuration management.
- `internal/server`: gRPC handlers for read and write operations.
- `internal/storage`: Core storage (Thread-safe Map) and persistence (WAL).

## Configuration

| Flag         | Environment Variable | Description                         | Type     | Default         |
| ------------ | -------------------- | ----------------------------------- | -------- | --------------- |
| `--addr`     | -                    | Server listening address            | String   | `:50051`        |
| `--peers`    | -                    | Comma-separated list of peers       | String   | -               |
| `--wal-path` | -                    | Path to the persistence log file    | String   | `data/kv.log`   |

## License

This project is licensed under the [MIT License](LICENSE).

<div align="center">

## Author

**Enoque Sousa**

[![LinkedIn](https://img.shields.io/badge/LinkedIn-0077B5?style=flat&logo=linkedin&logoColor=white)](https://www.linkedin.com/in/enoque-sousa-bb89aa168/)
[![GitHub](https://img.shields.io/badge/GitHub-100000?style=flat&logo=github&logoColor=white)](https://github.com/enoquesousa)
[![Portfolio](https://img.shields.io/badge/Portfolio-FF5722?style=flat&logo=target&logoColor=white)](https://enoquesousa.vercel.app)

**[⬆ Back to Top](#go-distributed-kv)**

Made with ❤️ by [Enoque Sousa](https://github.com/enoquesousa)

**Project Status:** Archived — Study Project

</div>
