package config

import (
	"errors"
	"flag"
	"reflect"
	"testing"
)

func TestParseDefaultsAndTLSPaths(t *testing.T) {
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:50051" || cfg.WALPath != "data/kv.log" || len(cfg.Peers) != 0 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	cfg, err = Parse([]string{"--addr", "node1:50051", "--peers", " node2:50052, [::1]:50053 ",
		"--tls-ca", "/certs/ca.crt", "--tls-cert", "/certs/node.crt", "--tls-key", "/certs/node.key"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Peers, []string{"node2:50052", "[::1]:50053"}) ||
		cfg.TLSCA != "/certs/ca.crt" || cfg.TLSCert != "/certs/node.crt" || cfg.TLSKey != "/certs/node.key" {
		t.Fatalf("incorrect parsed configuration: %+v", cfg)
	}
}

func TestParseRejectsIgnoredArgumentsAndEmptyPeers(t *testing.T) {
	for _, args := range [][]string{
		{"./godistributedkv", "--addr", ":50051"},
		{"--addr", "127.0.0.1:50051", "unexpected"},
		{"--peers", "127.0.0.1:50052,"},
		{"--peers", ",127.0.0.1:50052"},
		{"--peers", " "},
		{"--unknown"},
	} {
		if _, err := Parse(args); err == nil {
			t.Errorf("accepted invalid arguments: %q", args)
		}
	}
	if _, err := Parse([]string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("help error = %v", err)
	}
}
