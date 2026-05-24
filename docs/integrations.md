# Integrations

## Docker source

The Docker provider uses the Moby client with API version negotiation. It reads container snapshots and watches container and network events as hints for reconciliation.

### Label contract

- `proxy.aliases` defines aliases.
- `proxy.<alias>.port` declares a named alias.
- `proxy.#<n>.port` and `proxy.*.port` support indexed and wildcard ports.
- `proxy.<alias>.host`, `proxy.#<n>.host`, and `proxy.*.host` override the answer target.
- `proxy.exclude=true` opts a container out.
- If `base_domain` is configured on the source, bare aliases are expanded to FQDNs before reconciliation.

## AdGuard Home output

The AdGuard provider uses the control API under `/control/rewrite/*` with basic auth and JSON requests.

- List: `GET /control/rewrite/list`
- Add: `POST /control/rewrite/add`
- Update: `PUT /control/rewrite/update`
- Delete: `POST /control/rewrite/delete`

Temporary statuses treated as retryable include `408`, `429`, and `5xx` responses.

## Cloudflare output

The Cloudflare provider uses an API token and a zone ID.

- It lists DNS records for the zone and keeps hostnames as FQDNs in the daemon model.
- It caches record IDs for update/delete.
- It creates `A`, `AAAA`, or `CNAME` records based on the target value.
- It recovers duplicate-create error code `81058` by re-reading visible records.

## Upstream references

This repository’s source tree is the best reference for supported behavior:

- `internal/providers/docker/*`
- `internal/providers/adguard/*`
- `internal/providers/cloudflare/*`
