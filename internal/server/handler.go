// Package server implements the gRPC communication layer for the distributed
// key-value store. It handles client requests and internal cluster management
// protocols.
package server

import (
	"context"

	pb "github.com/enoquesousa/godistributedkv/api/proto"
	"github.com/enoquesousa/godistributedkv/internal/cluster"
	"github.com/enoquesousa/godistributedkv/internal/storage"
)

// Server implements the gRPC KeyValue service defined in the protobuf.
// It acts as the coordinator between the local [storage.Store] and the
// [cluster.Manager] for distributed operations.
type Server struct {
	pb.UnimplementedKeyValueServer
	store   *storage.Store
	cluster *cluster.Manager
	nodeID  string
}

// NewServer creates and returns a new [Server] instance fully configured
// with the provided store, cluster manager, and local node identifier.
func NewServer(store *storage.Store, cluster *cluster.Manager, nodeID string) *Server {
	return &Server{
		store:   store,
		cluster: cluster,
		nodeID:  nodeID,
	}
}

// Get handles the gRPC Get request by retrieving the value from the local [storage.Store].
// It returns a [pb.GetResponse] containing the value and existence status.
func (s *Server) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	val, found := s.store.Get(req.GetKey())
	return &pb.GetResponse{
		Value: val,
		Found: found,
	}, nil
}

// Set handles the gRPC Set request. Only the current cluster leader is permitted
// to perform write operations. If the node is not a leader, it returns a
// [pb.SetResponse] with a hint for the client to redirect to the correct leader.
func (s *Server) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetResponse, error) {
	if !s.cluster.IsLeader() {
		return &pb.SetResponse{
			Success:    false,
			LeaderHint: s.cluster.GetLeader(),
		}, nil
	}

	// Replicate to quorum via [cluster.Manager] before applying locally
	if !s.cluster.Replicate(ctx, req.GetKey(), req.GetValue()) {
		return &pb.SetResponse{
			Success: false,
		}, nil
	}

	if err := s.store.Set(req.GetKey(), req.GetValue()); err != nil {
		return &pb.SetResponse{Success: false}, err
	}
	return &pb.SetResponse{
		Success: true,
	}, nil
}

// Delete handles the gRPC Delete request for removing a key from the cluster.
// Similar to [Set], it requires the node to be the current leader and
// ensures quorum replication before local execution.
func (s *Server) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	if !s.cluster.IsLeader() {
		return &pb.DeleteResponse{
			Success: false,
		}, nil
	}

	// Replicate deletion state (represented by empty value) to quorum.
	if !s.cluster.Replicate(ctx, req.GetKey(), "") {
		return &pb.DeleteResponse{
			Success: false,
		}, nil
	}

	if err := s.store.Delete(req.GetKey()); err != nil {
		return &pb.DeleteResponse{Success: false}, err
	}
	return &pb.DeleteResponse{
		Success: true,
	}, nil
}

// Ping handles the gRPC health check request.
func (s *Server) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{
		NodeId:  s.nodeID,
		Healthy: true,
	}, nil
}

// RequestVote delegates election vote processing to the [cluster.Manager].
func (s *Server) RequestVote(ctx context.Context, req *pb.VoteRequest) (*pb.VoteResponse, error) {
	return s.cluster.HandleRequestVote(req), nil
}

// Heartbeat delegates leader heartbeat processing to the [cluster.Manager].
func (s *Server) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	return s.cluster.HandleHeartbeat(req), nil
}

// ReplicateSet handles internal replication requests initiated by the cluster leader.
// It ensures that follower state remains synchronized with the leader's authoritative log.
func (s *Server) ReplicateSet(ctx context.Context, req *pb.ReplicateRequest) (*pb.ReplicateResponse, error) {
	success, term := s.cluster.HandleReplicateSet(req)
	if success {
		var err error
		if req.GetValue() == "" {
			err = s.store.Delete(req.GetKey())
		} else {
			err = s.store.Set(req.GetKey(), req.GetValue())
		}
		if err != nil {
			return &pb.ReplicateResponse{Term: term, Success: false}, err
		}
	}
	return &pb.ReplicateResponse{
		Term:    term,
		Success: success,
	}, nil
}
