# Live Test Stack

This stack gives you a local AdGuard Home instance, the `docker-dns-sync` daemon under test, and one labeled sample workload.

The default config is AdGuard-only so the smoke test can run without any external DNS provider credentials.

## Services

- `adguardhome` exposes the admin/API UI on `http://127.0.0.1:13000`.
- `docker-dns-sync` watches the host Docker socket and syncs daemon-owned rewrites into AdGuard.
- `whoami` is a disposable labeled workload used only to drive managed rewrite creation in AdGuard; it is not published on a host port.
- `adguard-init` runs once to complete AdGuard's first-install flow automatically.

## Prerequisites

- `docker`
- `docker compose`
- `curl`
- Optional: `dig` for host-side DNS checks. If `dig` is missing, `deploy/compose/live-test/verify.sh` falls back to a containerized `nslookup` check.

## Automated run

Prefer the automated smoke test instead of manual `docker compose` steps:

```bash
mise run live-test
```

What it covers by default:

- initial managed rewrite creation
- managed rewrite update and restore
- manual-record safety across daemon restart
- restart recovery
- stale-delete verification is currently skipped in the default AdGuard harness because its visible rewrite API does not expose proof-bearing provenance

Cleanup defaults to `docker compose -f deploy/compose/live-test/compose.yaml down -v` on exit and removes the temporary runtime bind-mount directories. Keep the stack running only when you need to inspect it:

```bash
mise run live-test -- --keep-running
# or
KEEP_RUNNING=1 mise run live-test
```

In keep-running mode, the script preserves the temporary runtime directories that back the AdGuard and state bind mounts, then prints the exact `docker compose ... down -v` and `rm -rf ...` cleanup commands to run when you are done inspecting the stack.

Optional external outputs should be added only when you want to exercise them explicitly, and any secret must stay as an environment reference such as `api_key_ref = "ENV:CLOUDFLARE_API_KEY"`.

## Credentials and log hygiene

- The test stack uses fixed local-only AdGuard credentials inside the compose fixture.
- Do not paste real credentials inline when extending the stack or copying commands into docs.
- Do not echo secret environment values, enable `set -x`, or dump compose logs by default.
- If you need extra diagnostics, inspect only the specific service or API response relevant to the failure and avoid sharing secret-bearing config.

## Scope note

The live harness stays non-destructive unless the output can prove same-key ownership safely.

- The current live harness always skips the stale-delete step.
- AdGuard Home's visible rewrite API currently exposes only the hostname/answer pair, not a unique record identity or other stable provenance.
- Because same-key manual and managed rewrites are indistinguishable in that API, `mise run live-test` logs the stale-delete assertion as skipped instead of stopping the sample workload and risking a destructive delete.

This matches the reconcile safety rule: without ownership proof, stale cleanup must remain non-destructive.

## Why The Sample Labels Look Like This

The daemon does not derive a default answer target from a local Unix Docker socket endpoint. For local-socket testing, the sample workload needs an explicit host label, so the stack sets `proxy.whoami.test.host=127.0.0.1`.
