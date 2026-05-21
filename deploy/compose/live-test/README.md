# Live Test Stack

This stack gives you a local AdGuard Home instance, the `docker-dns-sync` daemon under test, and one labeled sample workload.

## Services

- `adguardhome` exposes the admin/API UI on `http://127.0.0.1:13000`.
- `docker-dns-sync` watches the host Docker socket and syncs daemon-owned rewrites into AdGuard.
- `whoami` is a disposable labeled workload exposed on `http://127.0.0.1:8080`.
- `adguard-init` runs once to complete AdGuard's first-install flow automatically.

## Credentials

- Username: `admin`
- Password: `adguard-test-password`

## Run

```bash
docker compose -f deploy/compose/live-test/compose.yaml up -d --build
docker compose -f deploy/compose/live-test/compose.yaml logs -f docker-dns-sync
```

## Verify

Check that the rewrite exists:

```bash
curl -u admin:adguard-test-password http://127.0.0.1:13000/control/rewrite/list
dig @127.0.0.1 -p 5353 whoami.test
```

Expected result: `whoami.test` should resolve to `127.0.0.1`.

## Why The Sample Labels Look Like This

The daemon does not derive a default answer target from a local Unix Docker socket endpoint. For local-socket testing, the sample workload needs an explicit host label, so the stack sets `proxy.whoami.test.host=127.0.0.1`.
