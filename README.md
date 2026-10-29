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

- **Go 1.26.10+**
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
# Node 1 (loopback-only development)
go run ./cmd/server --addr 127.0.0.1:50051 --peers 127.0.0.1:50052,127.0.0.1:50053 --wal-path data/node1.log

# Node 2
go run ./cmd/server --addr 127.0.0.1:50052 --peers 127.0.0.1:50051,127.0.0.1:50053 --wal-path data/node2.log

# Node 3
go run ./cmd/server --addr 127.0.0.1:50053 --peers 127.0.0.1:50051,127.0.0.1:50052 --wal-path data/node3.log
```

This mode has no authentication or encryption. The listener and every peer must
use a literal loopback IP, such as `127.0.0.1` or `[::1]`. Other local users and
processes can access the service. Hostnames (including `localhost`), wildcard
listeners such as `:50051`, and non-loopback peers require the mTLS configuration
below. Each process needs its own WAL file.

### Running with Docker Compose and mTLS

Generate development certificates with OpenSSL, then start the cluster:

```bash
./scripts/generate-dev-certs.sh
docker compose up --build
```

The script creates a fresh development CA, three node identities and a client
identity in the ignored `certs/dev/` directory. Certificates expire after seven
days, private files use owner-only permissions, and the script refuses to
overwrite an existing directory. These identities are for this local example;
use your own certificate management and dedicated CA for a deployed cluster.

Compose uses mTLS between containers and publishes the three gRPC ports only on
`127.0.0.1:60001`, `127.0.0.1:60002` and `127.0.0.1:60003`. Each container mounts
only the public CA and its own certificate/key, read-only. The CA private key is
never mounted. The `certs/` directory and `*.key`/`*.pem` files are excluded from
Git and the Docker build context. Keep the CA key offline when it is not being used.

Leader hints use the internal Compose node addresses. A client running on the
host must map a returned hint to the corresponding published endpoint; the
server does not translate these addresses automatically:

| Internal node ID / leader hint | Endpoint on the host |
| --- | --- |
| `node1:50051` | `127.0.0.1:60001` |
| `node2:50052` | `127.0.0.1:60002` |
| `node3:50053` | `127.0.0.1:60003` |

For example, a separately installed `grpcurl` can authenticate to node 1:

```bash
grpcurl -cacert certs/dev/ca.crt \
  -cert certs/dev/client.crt -key certs/dev/client.key \
  -d '{}' 127.0.0.1:60001 kv.KeyValue/Ping
```

Do not use `-plaintext` or `-insecure` for the mTLS example. For a non-Compose
deployment, supply `--tls-ca`, `--tls-cert` and `--tls-key` together on every node.
Node certificates must be valid for both server and client authentication,
because nodes also connect to peers. The example explicitly sets the
`serverAuth` and `clientAuth` extended key usages. Their SANs must match the hostnames or IPs
in `--peers` and in client connection targets. A node's listening address is also
its advertised node ID; use an address peers and clients can resolve and reach.

## Authentication and transport

The application has two explicit modes: unauthenticated loopback development,
or mutually authenticated TLS. It does not issue JWTs, refresh tokens, API keys
or user sessions.

| Component | Responsibility |
| --- | --- |
| `internal/config` | Reads listener, peers, WAL and certificate-file paths. |
| `internal/transport` | Validates the selected mode and loads TLS credentials and the explicit CA trust pool. |
| `cmd/server` | Configures the gRPC listener before starting cluster work. |
| `internal/cluster` | Uses the same transport policy for outbound peer connections. |
| `internal/server` | Runs key/value and consensus RPCs after the transport handshake. |

In mTLS mode, a connection must complete a TLS handshake before any RPC reaches
a handler. The server requires a client certificate valid for client
authentication under the configured CA; the client verifies the server's
certificate chain, validity and target SAN. Peer connections also present the
node's own certificate. There is no retry using plaintext after a TLS failure.
This protects all methods, including `Get`, `Set`, `Delete`, `Ping`,
`RequestVote`, `Heartbeat`, `ReplicateSet` and gRPC reflection.

Certificate trust is the authorization boundary: **every accepted client
certificate has access to all RPCs, including cluster administration and
replication**. There are no separate user/peer roles, per-key permissions or
certificate-to-node-ID bindings. Use a dedicated CA and issue certificates only
to trusted cluster operators and nodes; do not configure a broad public or
company-wide CA as the cluster's client trust root.

The process reads certificates and unencrypted PEM private keys from files at
startup and keeps the loaded private keys in memory. It does not send private keys over the network or
put them in logs or command-line arguments. File paths are configuration, not
credentials. Certificates and trust roots are not hot-reloaded, and there is no
application-level certificate revocation list. Replacing files requires a
restart; removing a compromised trust root requires a coordinated CA/identity
rotation and restart across the cluster. The WAL remains plaintext on disk, so
protect its filesystem separately.

The cluster is a study implementation of Raft-like consensus. mTLS protects the
connection and identifies members of the trust domain; it does not establish
production-grade consensus correctness or protect against a malicious trusted
member.

See the [gRPC authentication guide](https://grpc.io/docs/guides/auth/) for the
transport model and [SECURITY.md](SECURITY.md) for the security boundary.

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
- `internal/transport`: Loopback policy and mutual TLS credentials.
- `internal/server`: gRPC handlers for read and write operations.
- `internal/storage`: Core storage (Thread-safe Map) and persistence (WAL).

## Configuration

| Flag         | Environment Variable | Description                         | Type     | Default         |
| ------------ | -------------------- | ----------------------------------- | -------- | --------------- |
| `--addr`     | -                    | Server listening address and node ID | String  | `127.0.0.1:50051` |
| `--peers`    | -                    | Comma-separated list of peers       | String   | -               |
| `--wal-path` | -                    | Path to the persistence log file    | String   | `data/kv.log`   |
| `--tls-ca`   | -                    | PEM CA bundle for peer/client trust | String   | -               |
| `--tls-cert` | -                    | PEM node certificate/chain          | String   | -               |
| `--tls-key`  | -                    | Unencrypted PEM private key matching the certificate | String | - |

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
