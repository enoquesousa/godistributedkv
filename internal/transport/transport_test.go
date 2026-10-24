package transport

import "testing"

func TestPlaintextRequiresLiteralLoopbackForListenerAndEveryPeer(t *testing.T) {
	for _, address := range []string{"127.0.0.1:50051", "127.0.0.2:50051", "[::1]:50051", "[::ffff:127.0.0.1]:50051"} {
		t.Run(address, func(t *testing.T) {
			configured, err := New(address, []string{"127.0.0.1:50052", "[::1]:50053"}, TLSFiles{})
			if err != nil {
				t.Fatal(err)
			}
			options, err := configured.ServerOptions()
			if err != nil || len(options) != 0 {
				t.Fatalf("local plaintext server options = %v, %v", options, err)
			}
			if _, err := configured.ClientOptions(address); err != nil {
				t.Fatal(err)
			}
			if _, err := configured.ClientOptions("192.0.2.1:50051"); err == nil {
				t.Fatal("plaintext transport allowed a remote target after construction")
			}
		})
	}
	for _, address := range []string{
		":50051", "0.0.0.0:50051", "[::]:50051", "localhost:50051", "localhost.example:50051",
		"node1:50051", "192.0.2.1:50051", "[2001:db8::1]:50051", "127.1:50051",
		"127.0.0.1.example:50051", "dns:///127.0.0.1:50051", "passthrough:///127.0.0.1:50051",
		"127.0.0.1:bad", "127.0.0.1:65536", "127.0.0.1:-1", "127.0.0.1:+50051", "[::1%lo]:50051",
	} {
		t.Run("reject/"+address, func(t *testing.T) {
			if _, err := New(address, nil, TLSFiles{}); err == nil {
				t.Fatal("unsafe plaintext listener accepted")
			}
			if _, err := New("127.0.0.1:50051", []string{"127.0.0.1:50052", address}, TLSFiles{}); err == nil {
				t.Fatal("unsafe plaintext peer accepted")
			}
		})
	}
}

func TestTLSMustBeCompleteEvenOnLoopback(t *testing.T) {
	for _, files := range []TLSFiles{
		{CA: "ca.pem"}, {Cert: "node.pem"}, {Key: "node.key"},
		{CA: "ca.pem", Cert: "node.pem"}, {CA: "ca.pem", Key: "node.key"}, {Cert: "node.pem", Key: "node.key"},
	} {
		if _, err := New("127.0.0.1:50051", nil, files); err == nil {
			t.Fatal("partial TLS configuration accepted")
		}
	}
}

func TestUninitializedTransportFailsClosed(t *testing.T) {
	for _, configured := range []*Transport{nil, {}} {
		if _, err := configured.ServerOptions(); err == nil {
			t.Fatal("uninitialized transport allowed a server")
		}
		if _, err := configured.ClientOptions("127.0.0.1:50051"); err == nil {
			t.Fatal("uninitialized transport allowed a client")
		}
	}
}
