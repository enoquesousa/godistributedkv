// Package transport enforces the node's gRPC transport trust boundary.
// Remote connections require mutual TLS; plaintext is limited to literal loopback addresses.
package transport

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// TLSFiles names the PEM files used by a node in both its server and client roles.
// Values are filesystem paths, never inline keys or certificates.
type TLSFiles struct {
	// CA is the PEM trust bundle shared by the node's TLS server and client roles.
	CA string
	// Cert is this node's PEM certificate chain.
	Cert string
	// Key is the matching PEM private key.
	Key string
}

// Transport holds immutable credentials loaded before any storage or network services start.
// Its zero value is not intended for use; construct it with New.
type Transport struct {
	serverCredentials credentials.TransportCredentials
	clientCredentials credentials.TransportCredentials
	initialized       bool
}

// New validates every address and loads the complete TLS configuration, if supplied.
// Without TLS, the listener and every peer must use a literal loopback IP.
// A configured node certificate must be trusted by CA and valid for both TLS roles.
func New(addr string, peers []string, files TLSFiles) (*Transport, error) {
	hasTLS := files.CA != "" || files.Cert != "" || files.Key != ""
	if hasTLS && (files.CA == "" || files.Cert == "" || files.Key == "") {
		return nil, errors.New("TLS requires --tls-ca, --tls-cert and --tls-key together")
	}
	if err := validateAddress(addr, true, hasTLS); err != nil {
		return nil, err
	}
	for _, peer := range peers {
		if err := validateAddress(peer, false, hasTLS); err != nil {
			return nil, err
		}
	}
	if !hasTLS {
		return &Transport{initialized: true}, nil
	}

	// Only the explicitly configured bundle defines trust; no system roots are added.
	// #nosec G304 -- The CA path comes from trusted local operator configuration.
	caPEM, err := os.ReadFile(files.CA)
	if err != nil {
		return nil, errors.New("cannot read the TLS CA file")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("TLS CA file must contain PEM certificates")
	}
	certificate, err := tls.LoadX509KeyPair(files.Cert, files.Key)
	if err != nil {
		// Do not print file contents, key material, or accidental inline flag values.
		return nil, errors.New("cannot load the TLS certificate and matching private key from PEM files")
	}
	if err := validateNodeCertificate(certificate, roots); err != nil {
		return nil, err
	}

	serverTLS := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    roots,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
	clientTLS := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
		RootCAs:      roots,
		// ServerName is derived by gRPC from each target; there is no override.
	}
	return &Transport{
		serverCredentials: credentials.NewTLS(serverTLS),
		clientCredentials: credentials.NewTLS(clientTLS),
		initialized:       true,
	}, nil
}

// ServerOptions returns server credentials that protect every RPC, including reflection.
// No credentials are installed only for a validated local plaintext transport.
func (t *Transport) ServerOptions() ([]grpc.ServerOption, error) {
	if t == nil || !t.initialized {
		return nil, errors.New("transport must be initialized")
	}
	if t.serverCredentials == nil {
		return nil, nil
	}
	return []grpc.ServerOption{grpc.Creds(t.serverCredentials.Clone())}, nil
}

// ClientOptions returns credentials for a validated peer target.
// The address is checked again so a plaintext transport cannot dial a remote peer later.
func (t *Transport) ClientOptions(target string) ([]grpc.DialOption, error) {
	if t == nil || !t.initialized {
		return nil, errors.New("transport must be initialized")
	}
	if err := validateAddress(target, false, t.clientCredentials != nil); err != nil {
		return nil, err
	}
	if t.clientCredentials == nil {
		return []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, nil
	}
	return []grpc.DialOption{grpc.WithTransportCredentials(t.clientCredentials.Clone())}, nil
}

func validateAddress(address string, listener, hasTLS bool) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("addresses must use host:port, without a URI scheme")
	}
	if portErr := validatePort(port, listener); portErr != nil {
		return portErr
	}
	loopback, err := validateHost(host, listener)
	if err != nil {
		return err
	}
	if !hasTLS && !loopback {
		return errors.New("plaintext requires literal loopback IPs for the listener and every peer; configure mutual TLS for remote addresses")
	}
	return nil
}

func validatePort(port string, listener bool) error {
	for _, digit := range port {
		if digit < '0' || digit > '9' {
			return errors.New("addresses require a valid numeric TCP port")
		}
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 0 || portNumber > 65535 || (!listener && portNumber == 0) {
		return errors.New("addresses require a valid numeric TCP port")
	}
	return nil
}

// validateHost checks host syntax and reports whether it is a literal loopback IP.
func validateHost(host string, listener bool) (bool, error) {
	ip, ipErr := netip.ParseAddr(host)
	if ipErr == nil && ip.Zone() != "" {
		return false, errors.New("addresses must not include an IP zone")
	}
	if host == "" {
		if !listener {
			return false, errors.New("peer addresses require an explicit host")
		}
	} else if ipErr != nil && !validHostname(host) {
		return false, errors.New("addresses require an IP literal or DNS hostname, without a URI scheme")
	}
	return ipErr == nil && ip.IsLoopback(), nil
}

func validHostname(host string) bool {
	host = strings.TrimSuffix(host, ".")
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
				(character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func validateNodeCertificate(certificate tls.Certificate, roots *x509.CertPool) error {
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return errors.New("invalid TLS node certificate")
	}
	intermediates := x509.NewCertPool()
	for _, der := range certificate.Certificate[1:] {
		cert, parseErr := x509.ParseCertificate(der)
		if parseErr != nil {
			return errors.New("invalid TLS node certificate chain")
		}
		intermediates.AddCert(cert)
	}
	for _, usage := range []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth} {
		if _, verifyErr := leaf.Verify(x509.VerifyOptions{
			Roots: roots, Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{usage},
		}); verifyErr != nil {
			return errors.New("TLS node certificate must be current, trusted by --tls-ca, and valid for serverAuth and clientAuth")
		}
	}
	return nil
}
