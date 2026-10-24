package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUnsafeTransportFailsBeforeCreatingWAL(t *testing.T) {
	for name, args := range map[string][]string{
		"wildcard":       {"--addr", ":0"},
		"remote peer":    {"--addr", "127.0.0.1:0", "--peers", "192.0.2.1:50051"},
		"partial TLS":    {"--addr", "127.0.0.1:0", "--tls-key", "missing.key"},
		"unreadable TLS": {"--addr", "127.0.0.1:0", "--tls-ca", "missing.crt", "--tls-cert", "missing.crt", "--tls-key", "missing.key"},
		"positional":     {"./godistributedkv", "--addr", ":50051"},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state", "kv.log")
			arguments := append([]string{"--wal-path", path}, args...)
			result := make(chan error, 1)
			go func() { result <- run(arguments) }()
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("unsafe startup succeeded")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("unsafe startup blocked instead of rejecting configuration")
			}
			if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
				t.Fatalf("startup touched the WAL directory: %v", err)
			}
		})
	}
}
