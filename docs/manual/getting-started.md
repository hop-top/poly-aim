# Get started with aim

Install aim, refresh the local catalog, and run your first query.

## Use this page when

- First-time setup on a workstation or CI runner.
- Verifying an install works end-to-end.
- Looking for the smallest path to "hello, model".

## Result

After this guide, you will have:

- A working `aim` binary on `PATH`.
- A fresh local cache of the models.dev catalog.
- A successful `aim list` run.

## Before you begin

- Go 1.26+ installed (for `go install`), or download a release binary.
- Network access to `models.dev` for the first refresh.

## Quick version

```sh
go install hop.top/aim/cmd/aim@latest
aim refresh
aim list
```

## Steps

### 1. Install

```sh
go install hop.top/aim/cmd/aim@latest
```

Other install paths (release binaries, from source) live in
[install.md](install.md).

### 2. Refresh the cache

```sh
aim refresh
```

Fetches `https://models.dev/api.json` and writes it to the XDG cache.

### 3. List models

```sh
aim list
```

Expected: a table of provider/model rows.

### 4. Inspect one model

```sh
aim show openai gpt-4o
```

### 5. Verify the install

```sh
aim status --format json | jq '.sections[] | select(.title == "cache")'
```

Expected: a `cache` section with `last_fetch` set to a recent timestamp.

## Common issues

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `command not found: aim` | Go's bin not on `PATH` | add `$(go env GOPATH)/bin` to `PATH` |
| Empty list after refresh | source breaker tripped | `aim status` → check `source-breaker` state |
| `AIM_NETWORK` on refresh | offline or proxy in the way | retry, or see [troubleshooting.md](troubleshooting.md) |

## Next steps

- [Commands reference](commands.md) — every flag and subcommand.
- [Query syntax](query-syntax.md) — filter models with the DSL.
- [Drive aim from an agent](agent.md) — capability discovery, schema pinning.
- [Embed aim as a library](library.md) — programmatic API.
- [Configuration reference](configuration.md) — env vars and cache options.
- [Troubleshooting](troubleshooting.md) — recovery for common failures.
