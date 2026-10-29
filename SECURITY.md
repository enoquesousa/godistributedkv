# Security Policy

## Supported Versions

We only provide security updates for the following versions of **godistributedkv**:

| Version | Supported          |
| ------- | ------------------ |
| 1.0.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Security boundary

This repository is a study implementation. Its supported transport modes are:

- **Local development:** plaintext gRPC, restricted to literal loopback listener
  and peer IPs. Any local process can call its methods.
- **mTLS:** an explicit CA bundle and matching node certificate/private key are
  required together. Remote and wildcard addresses are rejected without this
  configuration. Both sides authenticate the TLS connection; server-name
  verification remains enabled for outbound connections.

All gRPC methods and reflection share this transport boundary. An accepted
client certificate grants full access, including consensus and replication
methods. There is no end-user login, token lifecycle, method-level role policy,
per-key ACL or certificate-to-node-ID authorization. Keep the client CA dedicated
to this cluster and restrict issuance to trusted participants.

PEM files are loaded at startup; the key loader expects unencrypted private-key
files. Protect private keys with filesystem access
controls, keep the issuing CA key outside server containers and exclude keys
from source control and image build contexts. The development script creates
seven-day identities and refuses to overwrite an existing directory. It is not
a production PKI or an automatic rotation service.

There is no certificate hot reload or application revocation list. Restart
processes after changing their keys or trust roots; coordinate a trust-root
rotation to remove a compromised identity from the trust domain. Existing TLS
connections also need to be terminated. Stored values and the WAL are not
encrypted at rest. Transport authentication does not correct the limitations of
the simplified consensus algorithm or prevent abuse by a trusted member.

## Reporting a Vulnerability

Please do not report vulnerabilities through public GitHub issues. Use the
repository's private vulnerability-reporting facility when available, and
include the vulnerability type, reproduction steps, potential impact and any
suggested mitigation. If private reporting is unavailable, establish a private
channel with the maintainer before sharing sensitive details. No functioning
security email address is configured in this repository.

Reports should omit usable credentials, private keys and customer data. The
maintainer verifies the report, coordinates a fix and release, and then arranges
public disclosure with attribution when requested.
