# Deployment

## Host binary with systemd

`deploy/systemd/docker-dns-sync.service` is the sample unit file.

```bash
mise exec -- go build -o bin/docker-dns-sync ./cmd/docker-dns-sync
sudo install -m 0755 bin/docker-dns-sync /usr/local/bin/docker-dns-sync
sudo install -m 0644 deploy/systemd/docker-dns-sync.service /etc/systemd/system/docker-dns-sync.service
sudo systemctl daemon-reload && sudo systemctl enable --now docker-dns-sync.service
```

The sample unit reads `/etc/docker-dns-sync/docker-dns-sync.env` and starts the binary with `-config /etc/docker-dns-sync/config.toml`.

## Container deployment

### Root compose example

`docker-compose.yaml` mounts `config.example.toml`, `./state`, and `/var/run/docker.sock`.

```bash
docker compose up -d
```

The image defaults to `ghcr.io/aaronflorey/docker-dns-sync:latest` unless `DOCKER_DNS_SYNC_IMAGE` is set.

### Dockerfile

`Dockerfile` builds a static binary on Go 1.26.3 and runs it from `gcr.io/distroless/static-debian12`.

### Release images and archives

`.goreleaser.yaml` publishes:

- Linux and macOS tarballs
- multi-arch GHCR images
- a Homebrew formula update

See `.github/workflows/release.yaml` for the publishing workflow.

## Deployment notes

- Keep the state directory persistent across restarts.
- Use a writable config path and a writable state path.
- Grant socket access only as broadly as your deployment requires.
