// Package main provides the entry point for the distributed key-value store server.
// It coordinates the initialization of storage, WAL persistence, and cluster
// management, then starts the gRPC server and enables reflection for debugging.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

	pb "github.com/enoquesousa/godistributedkv/api/proto"
	"github.com/enoquesousa/godistributedkv/internal/cluster"
	"github.com/enoquesousa/godistributedkv/internal/config"
	"github.com/enoquesousa/godistributedkv/internal/server"
	"github.com/enoquesousa/godistributedkv/internal/storage"
	"github.com/enoquesousa/godistributedkv/internal/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			config.Usage(os.Stdout)
			return
		}
		log.Fatalf("CRITICAL: %v", err)
	}
}

func run(args []string) error {
	// Reject unsafe addresses or credentials before touching WAL, listeners, or timers.
	cfg, err := config.Parse(args)
	if err != nil {
		return err
	}
	rpcTransport, err := transport.New(cfg.Addr, cfg.Peers, transport.TLSFiles{
		CA: cfg.TLSCA, Cert: cfg.TLSCert, Key: cfg.TLSKey,
	})
	if err != nil {
		return fmt.Errorf("invalid transport configuration: %w", err)
	}
	serverOptions, err := rpcTransport.ServerOptions()
	if err != nil {
		return err
	}
	log.Println("Starting godistributedkv...")
	log.Printf("Config loaded - Addr: %s, Peers: %s, WAL: %s",
		strconv.Quote(cfg.Addr), strconv.Quote(fmt.Sprint(cfg.Peers)), strconv.Quote(cfg.WALPath))

	// Initialize WAL.
	wal, err := storage.NewWAL(cfg.WALPath)
	if err != nil {
		return fmt.Errorf("failed to initialize WAL: %w", err)
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
	clusterMgr := cluster.NewManager(cfg.Addr, cfg.Peers, rpcTransport)

	// Initialize gRPC server.
	lis, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := grpc.NewServer(serverOptions...)
	srv := server.NewServer(kvStore, clusterMgr, cfg.Addr)
	pb.RegisterKeyValueServer(grpcServer, srv)

	// Enable reflection for tools like grpcurl.
	reflection.Register(grpcServer)

	clusterMgr.Start()
	log.Printf("gRPC server is listening at %v", lis.Addr())
	if err := grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}
	return nil
}
