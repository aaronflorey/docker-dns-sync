# docker-dns-sync

[![License](https://img.shields.io/github/license/aaronflorey/docker-dns-sync?style=flat-square)](LICENSE)
[![CI](https://img.shields.io/github/actions/workflow/status/aaronflorey/docker-dns-sync/ci.yaml?branch=main&style=flat-square&label=ci)](https://github.com/aaronflorey/docker-dns-sync/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/aaronflorey/docker-dns-sync?display_name=tag&style=flat-square)](https://github.com/aaronflorey/docker-dns-sync/releases)

`docker-dns-sync` is a Go daemon that watches Godoxy-labeled Docker workloads and keeps daemon-owned AdGuard Home DNS rewrites in sync.

## Installation

- Install from Homebrew with `brew install aaronflorey/tap/docker-dns-sync`.
- Download the latest release archive for your platform from GitHub Releases and place `docker-dns-sync` on your `PATH`.
- Build from source with `mise install && mise exec -- go build -o bin/docker-dns-sync ./cmd/docker-dns-sync`.
- Pull the release container image from `ghcr.io/aaronflorey/docker-dns-sync:<tag>`.

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

Start from `testdata/config/example.toml` for a host-binary deployment with a local Docker socket, `config.example.toml` or `testdata/config/docker-container.toml` for a Docker container deployment that mounts `/var/run/docker.sock`, or `testdata/config/socket-proxy.toml` for a TCP proxy or remote Docker source.

Required config sections:

- `[[sources]]` for Docker connectivity.
- `[[outputs]]` for AdGuard Home connectivity.
- `[state]` for the writable ownership snapshot path.
- `[logging]` for log level and format.
- `[retry]` for bounded backoff settings.

For Docker sources, set `sources[].host_ip` to the host-reachable IP address that unlabeled records should answer with. Explicit `proxy.<alias>.host`, `proxy.#<n>.host`, and `proxy.*.host` labels still override that shared source IP.

Use `password_ref = "ENV:ADGUARD_PASSWORD"` instead of embedding credentials in committed config.

## Development

- Install the pinned Go toolchain with `mise install`.
- Run static checks with `mise exec -- go vet ./...`.
- Run tests with `mise exec -- go test ./...`.
- Renovate is configured through `renovate.json` for Go modules, GitHub Actions, and container image references.

## Release Automation

- Pushes to `main` and `master` run CI and update the release-please PR.
- Merging the release PR creates a `vX.Y.Z` tag and GitHub release.
- Release publishing uploads macOS, Linux, and Windows archives plus checksums.
- The same tagged GoReleaser run publishes multi-arch container images to GHCR.
- The same tagged GoReleaser run updates the Homebrew formula in `aaronflorey/homebrew-tap`.
- The release workflow requires a `HOMEBREW_TAP_GITHUB_TOKEN` secret with push access to `aaronflorey/homebrew-tap`.

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

1. Pull a release image with `docker pull ghcr.io/aaronflorey/docker-dns-sync:<tag>`, or build locally with `docker build -t docker-dns-sync .`.
2. Use the root `docker-compose.yaml` plus `config.example.toml` for the simplest container deployment, or start from `testdata/config/docker-container.toml` / `testdata/config/socket-proxy.toml` for a custom deployment.
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
  ghcr.io/aaronflorey/docker-dns-sync:<tag> \
  -config /etc/docker-dns-sync/config.toml
```

Notes:

- The container image runs as root on purpose so the documented `/var/run/docker.sock` mount works with the default host socket ownership and mode used by Docker installations.
- For local Docker socket sources, set `host_ip` in the source config to the host-reachable address you want all derived records to answer with. Use `proxy.<alias>.host`, `proxy.#<n>.host`, or `proxy.*.host` only when a specific container needs a different answer target.
- `testdata/config/example.toml` is intentionally host-oriented and leaves AdGuard at `127.0.0.1`; do not reuse it unchanged inside the container unless AdGuard is reachable at container-local localhost.
- Mounting `/var/run/docker.sock` gives the container broad Docker control. If you do not want to grant that access, use a `tcp://...` endpoint such as `testdata/config/socket-proxy.toml` and give the container only the network path to that proxy or remote daemon.
- If your host uses rootless Docker or a non-standard socket owner, adjust the container user or socket permissions to match that environment before mounting the socket.
- Keep the state directory persistent across restarts so ownership recovery works.
- The container image runs the real CLI contract with `-config` and does not carry built-in config defaults.

## Example Files

- `testdata/config/example.toml` - env-ref based example config.
- `config.example.toml` - root-level compose-oriented example config.
- `docker-compose.yaml` - root-level container deployment example.
- `testdata/config/docker-container.toml` - env-ref based Docker container config for `/var/run/docker.sock` plus network-reachable AdGuard.
- `testdata/config/socket-proxy.toml` - env-ref based TCP proxy or remote Docker example config.
- `deploy/systemd/docker-dns-sync.service` - starting point for host deployment.
- `Dockerfile` - container build for Docker deployment.

## Contributing

See `CONTRIBUTING.md` for local setup and pull request expectations.

## License

Released under the MIT License. See `LICENSE`.
