# Development

## Setup

```bash
mise install
```

`mise.toml` pins Go 1.26.3, and `go.mod` pins the Go toolchain to `go1.26.3`.

## Common local commands

```bash
mise exec -- go build -o bin/docker-dns-sync ./cmd/docker-dns-sync
mise exec -- go vet ./...
mise exec -- go test ./...
docker build -t docker-dns-sync .
```

## Repository layout

- `cmd/docker-dns-sync` — CLI entrypoint.
- `internal/config` — config parsing and validation.
- `internal/providers` — Docker, AdGuard, and Cloudflare integrations.
- `internal/runtime` — reconciliation orchestration.
- `internal/state` — persisted snapshot handling.
- `testdata/config` — sample configs used in tests and docs.

## CI checks

The CI workflow runs:

- `go vet ./...`
- `go test ./...`
- `goreleaser ... check`
- `docker build -t docker-dns-sync-ci .`

See `.github/workflows/ci.yaml`.

## Notes

- There is no separate lint command defined in the repository.
- Keep config examples in sync with `testdata/config/` and `config.example.toml`.
