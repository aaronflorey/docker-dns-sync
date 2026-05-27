# Troubleshooting

## `-config` is missing

**Symptom**

```text
error: -config is required
```

**Cause**

The CLI has one required flag and does not use built-in defaults.

**Fix**

Run the daemon with `-config /path/to/config.toml`.

## Config file is missing, unreadable, or malformed

**Symptoms**

- `stat config file: ...`
- `decode config file: ...`

**Cause**

The path does not exist, cannot be read, or the TOML is invalid. The repository includes `testdata/config/malformed.toml` as a parse-failure example.

**Fix**

- Confirm the path exists and is readable by the process.
- Start from `testdata/config/example.toml`, `config.example.toml`, or another known-good sample.
- Validate the TOML syntax before restart.

## Invalid Docker endpoint scheme

**Symptom**

```text
sources[0].endpoint must use unix:// or tcp://
```

**Cause**

The source endpoint must match the validated Docker transport schemes.

**Fix**

Use a `unix://...` socket or a `tcp://...` endpoint.

## Credential ref problems

**Symptoms**

- `outputs[0] must set exactly one of password or password_ref`
- `outputs[0] must set exactly one of api_key or api_key_ref`
- `outputs[0].password_ref references an unset environment variable`
- `outputs[0].api_key_ref references an unset environment variable`

**Cause**

The config requires one credential source only, and `ENV:` references must resolve to a non-empty environment variable.

**Fix**

- Keep only one of the inline secret field or the `*_ref` field.
- Export the referenced environment variable before startup.

## Unsupported or stale state file

**Symptom**

```text
unsupported state snapshot version <n>
```

or a state load/decode error.

**Cause**

The snapshot format is versioned and the file may be stale, corrupted, or hand-edited.

**Fix**

- Point `state.path` at a fresh writable file.
- If the old file matters, back it up before replacing it.
- Deleting the file is safe; the store recreates an empty snapshot on startup when the path is missing.

## Ambiguous visible record collisions

**Symptom**

```text
visible record ambiguity for output <name> key <hostname>|<answer>: <n> matches
```

**Cause**

The output already contains more than one visible record for the same hostname/answer pair.

**Fix**

- Remove the duplicate records from the output system.
- Re-run the daemon after the visible set is clean.

## Docker event stream closes

**Symptom**

```text
docker event stream ended
```

or a reconnect warning in the logs.

**Cause**

The Docker event stream dropped or the daemon/socket/proxy became unavailable.

**Fix**

- Check Docker daemon health and socket permissions.
- Check TCP proxy connectivity if you use `tcp://...`.
- Re-run the daemon; it retries watch reconnects and full reconciliation.

## AdGuard request failures

**Symptom**

`adguard request ... failed with status ...`

**Cause**

The AdGuard API returned a non-2xx status or the request could not connect.

**Fix**

- Verify the URL and credentials.
- Check AdGuard availability.
- Inspect `curl -u user:pass <url>/control/rewrite/list` if you need a direct API check.

## Useful diagnostics

```bash
mise exec -- go test ./...
docker compose -f deploy/compose/live-test/compose.yaml logs -f docker-dns-sync
curl -u admin:adguard-test-password http://127.0.0.1:13000/control/rewrite/list
```

See also: [Configuration](configuration.md) and [Deployment](deployment.md).
