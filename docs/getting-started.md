# Getting started

## Prerequisites

- Go 1.26.3 via `mise` (`mise.toml` and `go.mod` pin the toolchain).
- Docker Engine access for the configured `sources[].endpoint`.
- AdGuard Home for the default documented output.
- Optional: Cloudflare API token if you enable a `cloudflare` output.
- For the live test stack: Docker Compose, `curl`, and `dig`.

## Install and bootstrap

1. Install the pinned toolchain:

   ```bash
   mise install
   ```

2. Build the daemon:

   ```bash
   mise exec -- go build -o bin/docker-dns-sync ./cmd/docker-dns-sync
   ```

3. Choose a config starter:

   - `testdata/config/example.toml` for a host binary with a local Docker socket.
   - `config.example.toml` or `testdata/config/docker-container.toml` for a container that mounts `/var/run/docker.sock`.
   - `testdata/config/socket-proxy.toml` for a TCP proxy or remote Docker endpoint.

4. Export any referenced secrets, for example:

   ```bash
   export ADGUARD_PASSWORD=replace-me
   ```

5. Run the daemon with an explicit config file:

   ```bash
   mise exec -- go run ./cmd/docker-dns-sync -config /path/to/config.toml
   ```

## Minimal verification path

The repository includes a self-contained live test stack under `deploy/compose/live-test/`:

```bash
docker compose -f deploy/compose/live-test/compose.yaml up -d --build
docker compose -f deploy/compose/live-test/compose.yaml logs -f docker-dns-sync
curl -u admin:adguard-test-password http://127.0.0.1:13000/control/rewrite/list
dig @127.0.0.1 -p 5353 whoami.test
```

Expected result: `whoami.test` resolves to `127.0.0.1`.

## Common next steps

- Tune `sources[].host_ip` and label overrides in [Configuration](configuration.md).
- Review [Integrations](integrations.md) for Docker, AdGuard Home, and Cloudflare details.
- Use [Deployment](deployment.md) for systemd or container-based installs.
