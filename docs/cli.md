# CLI

## Command

The repository ships one executable: `docker-dns-sync`.

### Usage

```bash
docker-dns-sync -config /path/to/config.toml
```

### Flags

| Flag | Required | Description |
| --- | --- | --- |
| `-config` | yes | Path to the TOML config file. |

## Exit behavior

- Missing `-config` exits with code 1.
- Config parsing, validation, and secret resolution errors are printed to stderr.
- The process handles `SIGINT` and `SIGTERM` through context cancellation.

## Related files

- `cmd/docker-dns-sync/main.go`
- `deploy/systemd/docker-dns-sync.service`
- `Dockerfile`
