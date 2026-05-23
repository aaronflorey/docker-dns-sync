# Security

## Secrets handling

- Use `password_ref = "ENV:NAME"` or `api_key_ref = "ENV:NAME"` instead of committing secrets into TOML.
- Secret references are resolved from process environment variables at startup.
- Unset or blank environment variables fail startup.

## Docker access

The Docker source can read from:

- a local Unix socket such as `/var/run/docker.sock`
- a `tcp://...` proxy or remote endpoint

Mounting the host Docker socket grants broad control over the Docker daemon. If you do not want that access, use a proxy or remote endpoint with narrower network permissions.

## Runtime hardening

The sample systemd unit sets:

- `NoNewPrivileges=true`
- a dedicated service user and group
- `Restart=on-failure`

The sample container image runs as root so the documented Docker socket mount works with common host socket ownership.

## What not to log

Do not log:

- AdGuard passwords
- Cloudflare API tokens
- environment variable values used by secret refs

The code resolves secrets before provider construction so the values can stay out of committed config.
