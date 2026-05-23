# Configuration

## Entry point

The daemon requires a config file path:

```bash
docker-dns-sync -config /path/to/config.toml
```

`cmd/docker-dns-sync/main.go` rejects startup if `-config` is missing.

## Config file structure

The TOML file must define these top-level sections:

- `[[sources]]`
- `[[outputs]]`
- `[state]`
- `[logging]`
- `[retry]`

The config is parsed first, then validated, then secret references are resolved from the process environment.

## Sources

| Key | Required | Notes |
| --- | --- | --- |
| `type` | yes | Registered source type. The default registry currently supports `docker`. |
| `name` | yes | Human-readable provider name used in logs and state. |
| `endpoint` | yes | Must start with `unix://` or `tcp://`. |
| `host_ip` | no | Default answer target when labels do not override it. Must be a valid IP address if set. |

### Docker source behavior

- `host_ip` is the preferred default answer target.
- If `host_ip` is empty and the endpoint is remote, the endpoint host is used as the default answer target.
- If the endpoint is a local socket (`unix://`), no default answer target is inferred.
- The source watches Docker container and network events and uses them as reconciliation hints.

## Outputs

| Key | Required | Notes |
| --- | --- | --- |
| `type` | yes | Supported values in the default registry: `adguard`, `cloudflare`. |
| `name` | yes | Human-readable provider name used in logs and state. |
| `enabled` | no | Defaults to `true` when omitted. Set `false` to skip the output. |

### AdGuard output

Required when `type = "adguard"` and enabled:

- `url`
- `username`
- exactly one of `password` or `password_ref`

`password_ref` must use the `ENV:` prefix, for example `ENV:ADGUARD_PASSWORD`.

### Cloudflare output

Required when `type = "cloudflare"` and enabled:

- `zone_id`
- exactly one of `api_key` or `api_key_ref`

`api_key_ref` must use the `ENV:` prefix, for example `ENV:CLOUDFLARE_API_KEY`.

## State

| Key | Required | Notes |
| --- | --- | --- |
| `path` | yes | Writable path for the owned-record snapshot. The store creates the file if it does not exist. |

State is written atomically with JSON encoding and `0600` permissions where supported.

## Logging

| Key | Allowed values | Notes |
| --- | --- | --- |
| `level` | `debug`, `info`, `warn`, `error` | Invalid values fail validation. |
| `format` | `json`, `text` | Output is written to stderr. |

## Retry

| Key | Notes |
| --- | --- |
| `initial_interval` | Required Go duration string, greater than zero. |
| `max_interval` | Required Go duration string, greater than zero. |
| `max_elapsed_time` | Required Go duration string, greater than zero. |

These values control bounded backoff during reconcile and watch-reconnect recovery.

## Label-driven record derivation

Docker sources interpret these labels:

- `proxy.exclude=true` disables the container.
- `proxy.aliases` is a comma-separated alias list.
- `proxy.<alias>.port` defines a named alias.
- `proxy.#<n>.port` and `proxy.*.port` provide indexed and wildcard ports.
- `proxy.<alias>.host`, `proxy.#<n>.host`, and `proxy.*.host` override the answer target.

Lookup precedence for host overrides is:

1. `proxy.<alias>.host`
2. `proxy.#<n>.host`
3. `proxy.*.host`

## Example starter configs

- `testdata/config/example.toml` — host-oriented example.
- `config.example.toml` — compose-oriented example.
- `testdata/config/docker-container.toml` — container deployment example.
- `testdata/config/socket-proxy.toml` — TCP proxy / remote Docker example.
