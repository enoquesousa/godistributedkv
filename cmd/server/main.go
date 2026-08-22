// Package main provides the entry point for the distributed key-value store server.
// It coordinates the initialization of storage, WAL persistence, and cluster
// management, then starts the gRPC server and enables reflection for debugging.
package main

import (
	"log"
	"net"

	pb "github.com/enoquesousa/godistributedkv/api/proto"
	"github.com/enoquesousa/godistributedkv/internal/cluster"
	"github.com/enoquesousa/godistributedkv/internal/config"
	"github.com/enoquesousa/godistributedkv/internal/server"
	"github.com/enoquesousa/godistributedkv/internal/storage"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	log.Println("Starting godistributedkv...")

	// Load configuration.
	cfg := config.Load()
	log.Printf("Config loaded - Addr: %s, Peers: %v, WAL: %s", cfg.Addr, cfg.Peers, cfg.WALPath)

	// Initialize WAL.
	wal, err := storage.NewWAL(cfg.WALPath)
	if err != nil {
		log.Fatalf("CRITICAL: failed to initialize WAL: %v", err)
	}
	defer func() {
		if cErr := wal.Close(); cErr != nil {
			log.Printf("ERROR: failed to close WAL: %v", cErr)
		}
	}()

	// Initialize in-memory storage with WAL.
	kvStore := storage.NewStore(wal)

	// Recover state from WAL.
	if rErr := kvStore.Recover(); rErr != nil {
		log.Printf("WARNING: failed to recover from WAL: %v", rErr)
	} else {
		log.Println("State recovered from WAL successfully")
	}

	// Initialize cluster manager.
	clusterMgr := cluster.NewManager(cfg.Addr, cfg.Peers)
	clusterMgr.Start()

	// Initialize gRPC server.
	lis, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Fatalf("CRITICAL: failed to listen on %s: %v", cfg.Addr, err)
	}

	grpcServer := grpc.NewServer()
	srv := server.NewServer(kvStore, clusterMgr, cfg.Addr)
	pb.RegisterKeyValueServer(grpcServer, srv)

	// Enable reflection for tools like grpcurl.
	reflection.Register(grpcServer)

	log.Printf("gRPC server is listening at %v", lis.Addr())
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("CRITICAL: failed to serve: %v", err)
	}
}
