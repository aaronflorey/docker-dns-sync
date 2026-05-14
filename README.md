# docker-dns-sync

`docker-dns-sync` is a Go daemon that watches Godoxy-labeled Docker workloads and keeps daemon-owned AdGuard Home DNS rewrites in sync.

## What It Does

- Performs a full reconcile on startup using Docker snapshot state and the persisted ownership file.
- Watches Docker for container lifecycle hints and reruns reconciliation after changes.
- Retries temporary AdGuard failures with bounded backoff before giving up.
- Only mutates rewrites that already belong to the daemon state file.

## Configuration

The binary contract is:

```text
docker-dns-sync -config /etc/docker-dns-sync/config.toml
```

Start from `testdata/config/example.toml` for a local Docker socket source or `testdata/config/socket-proxy.toml` for a TCP proxy or remote Docker source.

Required config sections:

- `[[sources]]` for Docker connectivity.
- `[[outputs]]` for AdGuard Home connectivity.
- `[state]` for the writable ownership snapshot path.
- `[logging]` for log level and format.
- `[retry]` for bounded backoff settings.

Use `password_ref = "ENV:ADGUARD_PASSWORD"` instead of embedding credentials in committed config.

## Host Binary With systemd

1. Build the binary with `mise exec -- go build -o bin/docker-dns-sync ./cmd/docker-dns-sync`.
2. Install the binary to `/usr/local/bin/docker-dns-sync`.
3. Copy `testdata/config/example.toml` to `/etc/docker-dns-sync/config.toml` for a local Docker socket source, or start from `testdata/config/socket-proxy.toml` if Docker is reachable through a proxy or remote endpoint. Keep a writable state path such as `/var/lib/docker-dns-sync/state.json`.
4. Create an environment file at `/etc/docker-dns-sync/docker-dns-sync.env` with `ADGUARD_PASSWORD=...`.
5. Install `deploy/systemd/docker-dns-sync.service` to `/etc/systemd/system/docker-dns-sync.service`.
6. Run `sudo systemctl daemon-reload && sudo systemctl enable --now docker-dns-sync.service`.

Notes:

- The sample unit intentionally does not require `docker.service`, so it also works when the Docker source uses `tcp://...` instead of the local socket.
- If you use `/var/run/docker.sock`, the service user must be able to read and write that socket. A common host setup is to add the service user to the group that owns the socket and uncomment `SupplementaryGroups=docker` in the sample unit if that group name is `docker`.
- The state file path must be writable by the service user.
- Restart behavior is explicit in the unit file: `Restart=on-failure`.

## Docker Deployment

1. Build the image with `docker build -t docker-dns-sync .`.
2. Copy `testdata/config/example.toml` to a host path such as `/opt/docker-dns-sync/config.toml` for a local socket deployment, or use `testdata/config/socket-proxy.toml` if the container should reach Docker through a TCP proxy or remote endpoint.
3. Create a writable host directory for state such as `/opt/docker-dns-sync/state`.
4. Provide the AdGuard password through an environment variable.
5. Run the container:

```bash
docker run -d \
  --name docker-dns-sync \
  --restart unless-stopped \
  -e ADGUARD_PASSWORD=replace-me \
  -v /opt/docker-dns-sync/config.toml:/etc/docker-dns-sync/config.toml:ro \
  -v /opt/docker-dns-sync/state:/var/lib/docker-dns-sync \
  -v /var/run/docker.sock:/var/run/docker.sock \
  docker-dns-sync \
  -config /etc/docker-dns-sync/config.toml
```

Notes:

- The container image runs as root on purpose so the documented `/var/run/docker.sock` mount works with the default host socket ownership and mode used by Docker installations.
- Mounting `/var/run/docker.sock` gives the container broad Docker control. If you do not want to grant that access, use a `tcp://...` endpoint such as `testdata/config/socket-proxy.toml` and give the container only the network path to that proxy or remote daemon.
- If your host uses rootless Docker or a non-standard socket owner, adjust the container user or socket permissions to match that environment before mounting the socket.
- Keep the state directory persistent across restarts so ownership recovery works.
- The container image runs the real CLI contract with `-config` and does not carry built-in config defaults.

## Example Files

- `testdata/config/example.toml` - env-ref based example config.
- `testdata/config/socket-proxy.toml` - env-ref based TCP proxy or remote Docker example config.
- `deploy/systemd/docker-dns-sync.service` - starting point for host deployment.
- `Dockerfile` - container build for Docker deployment.
