# Testing

## Default test command

```bash
mise exec -- go test ./...
```

The CI workflow uses the same command.

## Static verification

```bash
mise exec -- go vet ./...
```

## Integration-style local verification

Use the live test stack in `deploy/compose/live-test/`:

```bash
docker compose -f deploy/compose/live-test/compose.yaml up -d --build
docker compose -f deploy/compose/live-test/compose.yaml logs -f docker-dns-sync
curl -u admin:adguard-test-password http://127.0.0.1:13000/control/rewrite/list
dig @127.0.0.1 -p 5353 whoami.test
```

The expected resolution for `whoami.test` is `127.0.0.1`.

## Test fixtures

- `testdata/config/minimal.toml` — minimal runtime config.
- `testdata/config/example.toml` — host-oriented example.
- `testdata/config/socket-proxy.toml` — remote/proxy Docker example.
- `testdata/config/env-secret.toml` — secret-reference example.
- `testdata/config/docker-container.toml` — container deployment example.

## Adding tests

Prefer repository-level behavior tests for reconciliation, provider behavior, and config validation. The codebase already includes explicit tests around validation, provider factories, and live reconcile flows.
