# Contributing to godistributedkv

Thank you for your interest in contributing to **godistributedkv**! We welcome contributions from the community to help make this project even better.

## Development Environment

To set up your development environment, ensure you have the following installed:

- **Go 1.26.10+**: Keep the local toolchain aligned with CI and the Docker build.
- **Protocol Buffers (protoc)**: Required if you need to modify `.proto` files.
- **`protoc-gen-go` and `protoc-gen-go-grpc`**: Go plugins for protoc.
- **Make**: For running lifecycle commands.

### Setup

1.  Clone the repository:
    ```bash
    git clone https://github.com/enoquesousa/godistributedkv.git
    cd godistributedkv
    ```
2.  Install dependencies:
    ```bash
    make tidy
    ```

## Makefile Targets

The following commands are available to facilitate development:

| Target    | Description                                      |
|-----------|--------------------------------------------------|
| `build`   | Compiles the server binary into `bin/server`.    |
| `run`     | Runs the server directly using `go run`.         |
| `test`    | Executes all unit tests with verbose output.     |
| `tidy`    | Cleans up and updates `go.mod` and `go.sum`.     |
| `clean`   | Removes the `bin/` and `data/` directories.      |
| `protoc`  | Re-generates gRPC code from `api/proto/kv.proto`.|

## Code Style and Conventions

- **Standard Go Formatting**: Always run `gofmt` or `goimports` before committing.
- **Effective Go**: Follow the principles outlined in [Effective Go](https://go.dev/doc/effective_go).
- **Doc Comments**: Every exported function, type, and variable must have a doc comment following the [standard Go documentation style](https://go.dev/doc/comment).
- **Linting**: We use `golangci-lint`. Ensure your code passes all lint checks before submitting a PR.

## Testing

Always run tests before submitting a PR:
```bash
make test
```
If you introduce new features, please add corresponding unit tests in the relevant `_test.go` file.

For transport changes, run `go test -race ./...` and exercise both the loopback
policy and certificate rejection paths. Test identities must be generated or
clearly synthetic; never commit a real private key. See the README for the local
mTLS example. The certificate-generation script must not overwrite identities
that are already in use.

## Pull Request Process

1.  **Branching**: Create a new branch for your feature or bugfix (e.g., `feature/awesome-feature` or `fix/critical-bug`).
2.  **Commit Messages**: Write clear, concise commit messages. Prefix them with the scope (e.g., `feat:`, `fix:`, `docs:`, `ci:`).
3.  **Submit PR**: Open a Pull Request against the `master` branch.
4.  **Review**: At least one maintainer must review and approve your PR before it can be merged.

## Questions?

If you have any questions, feel free to open an issue or reach out to the project maintainers.
