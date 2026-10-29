package cluster

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/enoquesousa/godistributedkv/api/proto"
	"github.com/enoquesousa/godistributedkv/internal/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
)

func TestMutualTLSAuthenticatesAllRPCsAndReflection(t *testing.T) {
	authority := newTestAuthority(t)
	node := authority.issue(t, "node", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, false, false)
	rpcTransport, err := transport.New("127.0.0.1:0", nil, node.files)
	if err != nil {
		t.Fatal(err)
	}
	address, service := startTestServer(t, rpcTransport)
	options, err := rpcTransport.ClientOptions(address)
	if err != nil {
		t.Fatal(err)
	}
	connection := newTestConnection(t, address, options...)
	assertRPCs(t, connection, true)
	if service.calls.Load() != 7 {
		t.Fatalf("expected all seven RPCs, got %d", service.calls.Load())
	}
}

func TestMutualTLSRejectsUnauthorizedConnectionsBeforeRPCDispatch(t *testing.T) {
	authority := newTestAuthority(t)
	node := authority.issue(t, "node", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, false, false)
	rpcTransport, err := transport.New("127.0.0.1:0", nil, node.files)
	if err != nil {
		t.Fatal(err)
	}
	address, service := startTestServer(t, rpcTransport)
	otherAuthority := newTestAuthority(t)
	untrustedClient := otherAuthority.issue(t, "untrusted", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false, false)
	wrongRole := authority.issue(t, "server-only", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, false, false)
	expired := authority.issue(t, "expired", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, true, false)
	rootPool := x509.NewCertPool()
	rootPool.AddCert(authority.certificate)
	wrongPool := x509.NewCertPool()
	wrongPool.AddCert(otherAuthority.certificate)

	for _, test := range []struct {
		name  string
		creds credentials.TransportCredentials
	}{
		{"no client certificate", credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: rootPool})},
		{"untrusted client", credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: rootPool, Certificates: []tls.Certificate{untrustedClient.certificate}})},
		{"wrong client EKU", credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: rootPool, Certificates: []tls.Certificate{wrongRole.certificate}})},
		{"expired client", credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: rootPool, Certificates: []tls.Certificate{expired.certificate}})},
		{"untrusted server", credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: wrongPool, Certificates: []tls.Certificate{node.certificate}})},
		{"plaintext downgrade", insecure.NewCredentials()},
	} {
		t.Run(test.name, func(t *testing.T) {
			connection := newTestConnection(t, address, grpc.WithTransportCredentials(test.creds))
			assertRPCs(t, connection, false)
			if service.calls.Load() != 0 {
				t.Fatalf("unauthorized request reached a handler: %d", service.calls.Load())
			}
		})
	}
}

func TestPeerTransportVerifiesTargetSAN(t *testing.T) {
	authority := newTestAuthority(t)
	wrongName := authority.issue(t, "wrong-san", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, false, true)
	serverTransport, err := transport.New("127.0.0.1:0", nil, wrongName.files)
	if err != nil {
		t.Fatal(err)
	}
	address, service := startTestServer(t, serverTransport)
	node := authority.issue(t, "client-node", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, false, false)
	clientTransport, err := transport.New("127.0.0.1:0", []string{address}, node.files)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager("local", []string{address}, clientTransport)
	defer closeTestPeers(t, manager)
	client := manager.ensureClient(manager.peers[address])
	if client == nil {
		t.Fatal("client configuration failed before the handshake")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := client.Ping(ctx, &pb.PingRequest{}); err == nil {
		t.Fatal("a trusted certificate with the wrong SAN was accepted")
	}
	if service.calls.Load() != 0 {
		t.Fatal("wrong-SAN connection reached the service")
	}
}

func TestEnsureClientSharesAuthenticatedConnectionUnderConcurrency(t *testing.T) {
	authority := newTestAuthority(t)
	node := authority.issue(t, "node", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, false, false)
	rpcTransport, err := transport.New("127.0.0.1:0", nil, node.files)
	if err != nil {
		t.Fatal(err)
	}
	address, service := startTestServer(t, rpcTransport)
	manager := NewManager("local", []string{address}, rpcTransport)
	defer closeTestPeers(t, manager)
	peer := manager.peers[address]
	const requests = 24
	clients := make(chan pb.KeyValueClient, requests)
	var workers sync.WaitGroup
	for range requests {
		workers.Go(func() {
			client := manager.ensureClient(peer)
			clients <- client
			if client == nil {
				t.Error("authenticated peer client was not created")
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if _, callErr := client.Ping(ctx, &pb.PingRequest{}); callErr != nil {
				t.Errorf("authenticated peer call failed: %v", callErr)
			}
		})
	}
	workers.Wait()
	close(clients)
	for client := range clients {
		if client != peer.Client {
			t.Error("ensureClient returned different clients for the same peer")
		}
	}
	if service.calls.Load() != requests {
		t.Fatalf("handled %d requests, want %d", service.calls.Load(), requests)
	}
}

func TestLocalPlaintextRemainsAvailableWithoutTLS(t *testing.T) {
	rpcTransport, err := transport.New("127.0.0.1:0", nil, transport.TLSFiles{})
	if err != nil {
		t.Fatal(err)
	}
	address, _ := startTestServer(t, rpcTransport)
	options, err := rpcTransport.ClientOptions(address)
	if err != nil {
		t.Fatal(err)
	}
	assertRPCs(t, newTestConnection(t, address, options...), true)
}

func TestInvalidNodeCredentialsFailBeforeUse(t *testing.T) {
	authority := newTestAuthority(t)
	otherAuthority := newTestAuthority(t)
	valid := authority.issue(t, "valid", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, false, false)
	serverOnly := authority.issue(t, "server-only", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, false, false)
	clientOnly := authority.issue(t, "client-only", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false, false)
	expired := authority.issue(t, "expired", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, true, false)
	wrongCA := valid.files
	wrongCA.CA = otherAuthority.path
	mismatched := valid.files
	mismatched.Key = serverOnly.files.Key
	malformed := valid.files
	malformed.Key = filepath.Join(t.TempDir(), "malformed.key")
	const secretMarker = "SYNTHETIC_PRIVATE_KEY_MUST_NOT_APPEAR_IN_ERRORS"
	writeTestFile(t, malformed.Key, []byte(secretMarker))
	for name, files := range map[string]transport.TLSFiles{
		"wrong CA": wrongCA, "server only": serverOnly.files, "client only": clientOnly.files,
		"expired": expired.files, "mismatched key": mismatched, "malformed key": malformed,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := transport.New("node1:50051", []string{"node2:50052"}, files)
			if err == nil {
				t.Fatal("invalid node credentials were accepted")
			}
			if strings.Contains(err.Error(), secretMarker) || strings.Contains(err.Error(), "BEGIN PRIVATE KEY") {
				t.Fatal("credential contents leaked through a configuration error")
			}
		})
	}
	if _, err := transport.New("node1:50051", []string{"node2:50052"}, valid.files); err != nil {
		t.Fatalf("valid remote configuration rejected: %v", err)
	}
}

func assertRPCs(t *testing.T, connection *grpc.ClientConn, allowed bool) {
	t.Helper()
	client := pb.NewKeyValueClient(connection)
	for name, call := range map[string]func(context.Context) error{
		"Get": func(ctx context.Context) error { _, err := client.Get(ctx, &pb.GetRequest{Key: "key"}); return err },
		"Set": func(ctx context.Context) error {
			_, err := client.Set(ctx, &pb.SetRequest{Key: "key", Value: "value"})
			return err
		},
		"Delete": func(ctx context.Context) error {
			_, err := client.Delete(ctx, &pb.DeleteRequest{Key: "key"})
			return err
		},
		"Ping": func(ctx context.Context) error { _, err := client.Ping(ctx, &pb.PingRequest{}); return err },
		"RequestVote": func(ctx context.Context) error {
			_, err := client.RequestVote(ctx, &pb.VoteRequest{Term: 1})
			return err
		},
		"Heartbeat": func(ctx context.Context) error {
			_, err := client.Heartbeat(ctx, &pb.HeartbeatRequest{Term: 1})
			return err
		},
		"ReplicateSet": func(ctx context.Context) error {
			_, err := client.ReplicateSet(ctx, &pb.ReplicateRequest{Term: 1, Key: "key", Value: "value"})
			return err
		},
		"reflection": func(ctx context.Context) error {
			stream, err := reflectionpb.NewServerReflectionClient(connection).ServerReflectionInfo(ctx)
			if err != nil {
				return err
			}
			if sendErr := stream.Send(&reflectionpb.ServerReflectionRequest{MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{ListServices: ""}}); sendErr != nil {
				return sendErr
			}
			_, err = stream.Recv()
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := call(ctx)
			if allowed && err != nil {
				t.Fatalf("authenticated RPC failed: %v", err)
			}
			if !allowed && err == nil {
				t.Fatal("unauthorized RPC succeeded")
			}
		})
	}
}

func startTestServer(t *testing.T, rpcTransport *transport.Transport) (string, *testRPCServer) {
	t.Helper()
	options, err := rpcTransport.ServerOptions()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &testRPCServer{}
	grpcServer := grpc.NewServer(options...)
	pb.RegisterKeyValueServer(grpcServer, service)
	reflection.Register(grpcServer)
	result := make(chan error, 1)
	go func() { result <- grpcServer.Serve(listener) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		if serveErr := <-result; serveErr != nil {
			t.Errorf("test gRPC server: %v", serveErr)
		}
	})
	return listener.Addr().String(), service
}

func newTestConnection(t *testing.T, address string, options ...grpc.DialOption) *grpc.ClientConn {
	t.Helper()
	connection, err := grpc.NewClient(address, options...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := connection.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	return connection
}

func closeTestPeers(t *testing.T, manager *Manager) {
	t.Helper()
	for _, peer := range manager.peers {
		if peer.Conn != nil {
			if err := peer.Conn.Close(); err != nil {
				t.Error(err)
			}
		}
	}
}

type testRPCServer struct {
	pb.UnimplementedKeyValueServer
	calls atomic.Int64
}

func (s *testRPCServer) Get(context.Context, *pb.GetRequest) (*pb.GetResponse, error) {
	s.calls.Add(1)
	return &pb.GetResponse{Found: true, Value: "value"}, nil
}

func (s *testRPCServer) Set(context.Context, *pb.SetRequest) (*pb.SetResponse, error) {
	s.calls.Add(1)
	return &pb.SetResponse{Success: true}, nil
}

func (s *testRPCServer) Delete(context.Context, *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	s.calls.Add(1)
	return &pb.DeleteResponse{Success: true}, nil
}

func (s *testRPCServer) Ping(context.Context, *pb.PingRequest) (*pb.PingResponse, error) {
	s.calls.Add(1)
	return &pb.PingResponse{Healthy: true, NodeId: "test"}, nil
}

func (s *testRPCServer) RequestVote(context.Context, *pb.VoteRequest) (*pb.VoteResponse, error) {
	s.calls.Add(1)
	return &pb.VoteResponse{VoteGranted: true}, nil
}

func (s *testRPCServer) Heartbeat(context.Context, *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	s.calls.Add(1)
	return &pb.HeartbeatResponse{Success: true}, nil
}

func (s *testRPCServer) ReplicateSet(context.Context, *pb.ReplicateRequest) (*pb.ReplicateResponse, error) {
	s.calls.Add(1)
	return &pb.ReplicateResponse{Success: true}, nil
}

type testAuthority struct {
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
	path        string
}

type testCertificate struct {
	certificate tls.Certificate
	files       transport.TLSFiles
}

func newTestAuthority(t *testing.T) testAuthority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	writeTestFile(t, path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return testAuthority{certificate: certificate, key: key, path: path}
}

func (authority testAuthority) issue(t *testing.T, name string, usages []x509.ExtKeyUsage, expired, wrongSAN bool) testCertificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: usages,
		DNSNames: []string{"localhost", "node1", "node2"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	if expired {
		template.NotAfter = time.Now().Add(-time.Minute)
	}
	if wrongSAN {
		template.DNSNames = []string{"different.example.test"}
		template.IPAddresses = nil
	}
	der, err := x509.CreateCertificate(rand.Reader, template, authority.certificate, &key.PublicKey, authority.key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	directory := t.TempDir()
	files := transport.TLSFiles{CA: authority.path, Cert: filepath.Join(directory, "node.pem"), Key: filepath.Join(directory, "node.key")}
	writeTestFile(t, files.Cert, certPEM)
	writeTestFile(t, files.Key, keyPEM)
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return testCertificate{certificate: certificate, files: files}
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
