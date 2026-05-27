# Data model

## State snapshot

The persisted snapshot lives at `state.path` and is JSON encoded.

### Schema

```json
{
  "version": 1,
  "managed_records": [
    {
      "output": { "Type": "adguard", "Name": "primary-adguard" },
      "source": {
        "Provider": { "Type": "docker", "Name": "local-docker" },
        "ID": "...",
        "DisplayName": "whoami"
      },
      "hostname": "whoami.test",
      "answer": "127.0.0.1",
      "last_applied_at": "2026-05-23T00:00:00Z"
    }
  ]
}
```

The top-level snapshot fields and `ManagedRecord` fields are snake_case because they define explicit JSON tags.
The nested provider/source reference structs use Go's default exported field names because they do not define JSON tags.

### Fields

| Field | Notes |
| --- | --- |
| `version` | Snapshot format version. Current value: `1`. |
| `managed_records` | Managed output records owned by the daemon. |
| `output` | Output provider reference. |
| `source` | Source object reference. |
| `hostname` | Managed hostname. |
| `answer` | Managed answer target. |
| `last_applied_at` | Time the record was last applied. |

## Persistence behavior

- The store creates the file if it does not exist.
- Writes are atomic.
- The file is saved with `0600` permissions where supported.
- A mismatched snapshot version is rejected instead of silently migrated.

## Where it is used

- `internal/state/model.go`
- `internal/state/store.go`
- `internal/runtime/reconcile.go`
- `internal/runtime/reconcile_apply.go`
