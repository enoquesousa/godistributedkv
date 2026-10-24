// Package config handles the application configuration via command-line flags.
package config

import (
	"errors"
	"flag"
	"io"
	"strings"
)

// Config holds the application configuration parameters.
type Config struct {
	// Addr is the network address (host:port) where the server will listen.
	Addr string
	// Peers is a list of network addresses for other nodes in the cluster.
	Peers []string
	// WALPath is the filesystem path to the Write-Ahead Log file.
	WALPath string
	// TLSCA is the path to the PEM trust bundle for both node and client certificates.
	TLSCA string
	// TLSCert is the path to this node's PEM certificate chain, usable for server and client authentication.
	TLSCert string
	// TLSKey is the path to the PEM private key matching TLSCert.
	TLSKey string
}

// Parse parses command-line arguments without opening files or starting services.
// Transport validation and credential loading must complete before using the result.
func Parse(args []string) (*Config, error) {
	cfg, peersStr, flags := configurationFlags()
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	if flags.NArg() != 0 {
		return nil, errors.New("unexpected positional arguments: use server flags only")
	}

	if *peersStr != "" {
		for _, peer := range strings.Split(*peersStr, ",") {
			peer = strings.TrimSpace(peer)
			if peer == "" {
				return nil, errors.New("peer addresses must not be empty")
			}
			cfg.Peers = append(cfg.Peers, peer)
		}
	}
	return cfg, nil
}

// Usage writes the supported command-line flags and their defaults.
func Usage(output io.Writer) {
	_, _, flags := configurationFlags()
	flags.SetOutput(output)
	flags.PrintDefaults()
}

func configurationFlags() (*Config, *string, *flag.FlagSet) {
	cfg := &Config{}
	flags := flag.NewFlagSet("godistributedkv", flag.ContinueOnError)
	flags.StringVar(&cfg.Addr, "addr", "127.0.0.1:50051", "Server listen address (host:port)")
	peers := flags.String("peers", "", "Comma-separated peer addresses (host:port)")
	flags.StringVar(&cfg.WALPath, "wal-path", "data/kv.log", "Path to the Write-Ahead Log file")
	flags.StringVar(&cfg.TLSCA, "tls-ca", "", "Path to the PEM CA trust bundle (requires tls-cert and tls-key)")
	flags.StringVar(&cfg.TLSCert, "tls-cert", "", "Path to this node's PEM certificate chain (serverAuth and clientAuth)")
	flags.StringVar(&cfg.TLSKey, "tls-key", "", "Path to this node's PEM private key")
	return cfg, peers, flags
}
