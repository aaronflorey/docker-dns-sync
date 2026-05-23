# docker-dns-sync documentation

`docker-dns-sync` is a Go daemon that watches Docker workloads, derives DNS records from Godoxy-style labels, and reconciles daemon-owned records into configured outputs. The primary documented operator path is AdGuard Home rewrites; the codebase also supports Cloudflare DNS as an output.

## What this docs set covers

- [Getting started](getting-started.md)
- [Configuration](configuration.md)
- [Architecture](architecture.md)
- [CLI](cli.md)
- [Development](development.md)
- [Testing](testing.md)
- [Deployment](deployment.md)
- [Integrations](integrations.md)
- [Data model](data-model.md)
- [Security](security.md)
- [Troubleshooting](troubleshooting.md)

## Recommended reading order

1. Start with [Getting started](getting-started.md).
2. Read [Configuration](configuration.md) before editing any TOML.
3. Review [Architecture](architecture.md) if you need to understand reconciliation or recovery.
4. Use [Testing](testing.md) and [Development](development.md) for local workflow.
5. Use [Deployment](deployment.md) and [Integrations](integrations.md) for operator setup.
6. Keep [Troubleshooting](troubleshooting.md) handy for startup and runtime failures.

## Known limitations

- The binary has no built-in config defaults; `-config` is required.
- Only source/output types registered in code are supported: `docker` sources, plus `adguard` and `cloudflare` outputs.
- Local-socket Docker sources do not infer a default answer target unless `host_ip` or explicit `proxy.*.host` labels are provided.
