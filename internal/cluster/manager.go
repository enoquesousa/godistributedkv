// Package cluster provides cluster management capabilities, including
// service discovery, leader election, and distributed replication.
//
// It implements a Raft-lite consensus mechanism to ensure high availability
// and data consistency across multiple nodes.
package cluster

import (
	"context"
	"crypto/rand"
	"log"
	"math/big"
	"sync"
	"time"

	pb "github.com/enoquesousa/godistributedkv/api/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// NodeState represents the current role of a node in the cluster.
type NodeState int

const (
	// Follower is the default state of a node. It responds to requests from leaders and candidates.
	Follower NodeState = iota
	// Candidate represents a node that is currently attempting to become a leader.
	Candidate
	// Leader represents the node that is currently authoritative for the cluster.
	Leader
)

// Peer represents a remote node in the cluster.
// It maintains the network address and gRPC client connection.
type Peer struct {
	// Addr is the network address (host:port) of the remote peer.
	Addr string
	// Client is the gRPC client for the KeyValue service.
	Client pb.KeyValueClient
	// Conn is the underlying gRPC client connection.
	Conn *grpc.ClientConn
}

// Manager coordinates cluster-wide operations such as leader election and replication.
// It tracks the state of the local node and its relationship with [Peer] nodes.
type Manager struct {
	mu sync.RWMutex

	// Node Info
	nodeID string
	peers  map[string]*Peer

	// Raft-lite State
	state       NodeState
	currentTerm int64
	votedFor    string
	leaderID    string

	// Timers
	electionTimer *time.Timer
}

// NewManager initializes and returns a new cluster [Manager].
func NewManager(nodeID string, peerAddrs []string) *Manager {
	m := &Manager{
		nodeID:      nodeID,
		peers:       make(map[string]*Peer),
		state:       Follower,
		currentTerm: 0,
	}

	for _, addr := range peerAddrs {
		m.peers[addr] = &Peer{
			Addr: addr,
		}
	}

	return m
}

// Start initiates the election timer and background cluster processes.
// It should be called after initializing all dependencies.
func (m *Manager) Start() {
	m.mu.Lock()
	m.resetElectionTimer()
	m.mu.Unlock()
}

func (m *Manager) resetElectionTimer() {
	if m.electionTimer != nil {
		m.electionTimer.Stop()
	}
	// Randomized timeout between 150ms and 300ms to reduce split-vote scenarios.
	// Use crypto/rand to comply with professional security standards.
	maxJitter := big.NewInt(150)
	jitter, err := rand.Int(rand.Reader, maxJitter)
	if err != nil {
		// Fallback to fixed 300ms if random generator fails (extremely unlikely)
		log.Printf("CRITICAL: crypto/rand failed: %v", err)
		jitter = big.NewInt(150)
	}

	timeout := time.Duration(150+jitter.Int64()) * time.Millisecond
	m.electionTimer = time.AfterFunc(timeout, m.startElection)
}

// startElection transitions the node to the [Candidate] state and requests votes from [Peer] nodes.
func (m *Manager) startElection() {
	m.mu.Lock()
	m.state = Candidate
	m.currentTerm++
	m.votedFor = m.nodeID
	m.leaderID = ""
	term := m.currentTerm
	nodeID := m.nodeID
	m.resetElectionTimer()
	log.Printf("[TERM %d] Node %s starting election", term, nodeID)
	m.mu.Unlock()

	votes := 1 // Vote for self
	var voteMu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range m.peers {
		wg.Add(1)
		go func(peer *Peer) {
			defer wg.Done()
			m.ensureClient(peer)
			if peer.Client == nil {
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			resp, err := peer.Client.RequestVote(ctx, &pb.VoteRequest{
				Term:        term,
				CandidateId: nodeID,
			})

			if err != nil {
				return
			}

			if resp.VoteGranted {
				voteMu.Lock()
				votes++
				voteMu.Unlock()
			} else if resp.Term > term {
				m.mu.Lock()
				m.becomeFollower(resp.Term)
				m.mu.Unlock()
			}
		}(p)
	}

	wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if still candidate and has reached majority quorum.
	if m.state == Candidate && votes > (len(m.peers)+1)/2 {
		m.becomeLeader()
	}
}

// becomeLeader transitions the node to the [Leader] state and starts the heartbeat loop.
func (m *Manager) becomeLeader() {
	m.state = Leader
	m.leaderID = m.nodeID
	if m.electionTimer != nil {
		m.electionTimer.Stop()
	}
	log.Printf("[TERM %d] Node %s became LEADER", m.currentTerm, m.nodeID)
	go m.heartbeatLoop()
}

// becomeFollower transitions the node to the [Follower] state for the specified term.
func (m *Manager) becomeFollower(term int64) {
	log.Printf("[TERM %d] Node %s becoming Follower (Term update: %d)", m.currentTerm, m.nodeID, term)
	m.state = Follower
	m.currentTerm = term
	m.votedFor = ""
	m.resetElectionTimer()
}

// heartbeatLoop sends periodic heartbeats to all [Peer] nodes to maintain leadership authority.
func (m *Manager) heartbeatLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		m.mu.RLock()
		if m.state != Leader {
			m.mu.RUnlock()
			return
		}
		term := m.currentTerm
		nodeID := m.nodeID
		m.mu.RUnlock()

		for _, p := range m.peers {
			go func(peer *Peer) {
				m.ensureClient(peer)
				if peer.Client == nil {
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
				defer cancel()
				resp, err := peer.Client.Heartbeat(ctx, &pb.HeartbeatRequest{
					Term:     term,
					LeaderId: nodeID,
				})
				if err == nil && resp.Term > term {
					m.mu.Lock()
					m.becomeFollower(resp.Term)
					m.mu.Unlock()
				}
			}(p)
		}
		<-ticker.C
	}
}

func (m *Manager) ensureClient(peer *Peer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if peer.Conn == nil {
		conn, err := grpc.NewClient(peer.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return
		}
		peer.Conn = conn
		peer.Client = pb.NewKeyValueClient(conn)
	}
}

// HandleRequestVote processes a vote request from a candidate in the cluster.
func (m *Manager) HandleRequestVote(req *pb.VoteRequest) *pb.VoteResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	if req.Term > m.currentTerm {
		m.becomeFollower(req.Term)
	}

	granted := false
	if req.Term == m.currentTerm && (m.votedFor == "" || m.votedFor == req.CandidateId) {
		granted = true
		m.votedFor = req.CandidateId
		m.resetElectionTimer()
	}

	return &pb.VoteResponse{
		Term:        m.currentTerm,
		VoteGranted: granted,
	}
}

// HandleHeartbeat processes a heartbeat from the current cluster leader.
func (m *Manager) HandleHeartbeat(req *pb.HeartbeatRequest) *pb.HeartbeatResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	if req.Term > m.currentTerm {
		m.becomeFollower(req.Term)
	}

	success := false
	if req.Term >= m.currentTerm {
		success = true
		m.state = Follower
		m.currentTerm = req.Term
		m.leaderID = req.LeaderId
		m.resetElectionTimer()
	}

	return &pb.HeartbeatResponse{
		Term:    m.currentTerm,
		Success: success,
	}
}

// IsLeader returns true if the local node is currently the cluster leader.
func (m *Manager) IsLeader() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state == Leader
}

// GetLeader returns the ID of the current cluster leader, if known.
func (m *Manager) GetLeader() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.leaderID
}

// Replicate orchestrates the replication of a Set operation to a quorum of cluster peers.
// It returns true if the operation reached a majority of nodes.
func (m *Manager) Replicate(ctx context.Context, key, value string) bool {
	m.mu.RLock()
	if m.state != Leader {
		m.mu.RUnlock()
		return false
	}
	term := m.currentTerm
	nodeID := m.nodeID
	peers := make([]*Peer, 0, len(m.peers))
	for _, p := range m.peers {
		peers = append(peers, p)
	}
	m.mu.RUnlock()

	successCount := 1 // Leader itself
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range peers {
		wg.Add(1)
		go func(peer *Peer) {
			defer wg.Done()
			m.ensureClient(peer)
			if peer.Client == nil {
				return
			}

			// Individual peer timeout
			peerCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			defer cancel()

			resp, err := peer.Client.ReplicateSet(peerCtx, &pb.ReplicateRequest{
				Term:     term,
				LeaderId: nodeID,
				Key:      key,
				Value:    value,
			})

			if err == nil && resp.Success {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(p)
	}

	wg.Wait()

	totalNodes := len(peers) + 1
	majority := (totalNodes / 2) + 1
	return successCount >= majority
}

// HandleReplicateSet processes a replication request from the cluster leader.
func (m *Manager) HandleReplicateSet(req *pb.ReplicateRequest) (bool, int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if req.Term < m.currentTerm {
		return false, m.currentTerm
	}

	if req.Term > m.currentTerm {
		m.becomeFollower(req.Term)
	}

	m.leaderID = req.LeaderId
	m.resetElectionTimer()

	return true, m.currentTerm
}
