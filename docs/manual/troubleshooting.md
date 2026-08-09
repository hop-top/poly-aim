# Troubleshoot aim

## Use this page when

- A command failed and you want a stable code → fix mapping.
- `aim list` returns nothing.
- An error envelope is surprising.

## Start with `aim status`

Most issues resolve from one command:

```sh
aim status --format json
```

It dumps cache state, source URL, identity, paths, and the source-fetch
circuit breaker. If `aim status` itself crashes, the problem is
install-level — see [install.md](install.md).

## Fix common issues

### Fix `command not found: aim`

```sh
which aim
aim --version
```

If missing, reinstall (see [install.md](install.md)). If `aim` is
installed but not on `PATH`, add `$(go env GOPATH)/bin` to `PATH`.

### Fix empty `aim list` / "no models found"

The cache may be empty or stale. Force a refresh:

```sh
aim refresh --force
aim list
```

#### If only `--provider` / `provider:` filters come back empty

Releases before 0.1.0-alpha.3 dropped the provider field when writing the
cache to disk, so any provider filter matched nothing from the second run
onward — the first run after a cold cache worked, every later one returned
zero results. `aim refresh --force` did **not** help, because the refresh
rewrote the cache in the same lossy shape.

Upgrade to 0.1.0-alpha.3 or later (see [install.md](install.md)); the fix
re-derives the provider on load, so existing cache files start working
without any manual step. To confirm which side of the fix you are on:

```sh
aim list --format json | jq -e '.data[0].provider != ""'
```

If `aim refresh` errors with `AIM_NETWORK` or `AIM_SOURCE_UNAVAILABLE`,
check connectivity and the source breaker state:

```sh
aim status --format json | jq '.sections[] | select(.title == "source-breaker")'
```

A breaker in the `open` state fails fast for 30 seconds after three
consecutive transport errors. Wait for it to half-open, or run on a
different network.

### Fix stale data

Default TTL is 24 hours. Force a refresh:

```sh
aim refresh --force
```

Use `aim refresh --dry-run --force` to preview without making the
network call.

### Decode an unexpected error envelope

Every error returns a stable code on stderr. Inspect with:

```sh
aim show nope nope --format json 2>&1 >/dev/null
```

The `code`, `suggested_fix`, and `alternatives` fields are deliberately
runnable. The full code catalog is in [agent.md](agent.md#error-envelope)
and [`internal/errs/errs.go`](../../internal/errs/errs.go).

### Force a specific output format

Defaults are `table` on TTY, `json` otherwise. Override:

```sh
aim list --format json
```

### Quiet hints in scripted output

```sh
aim list --no-hints
```

Hints auto-suppress when stdout is not a TTY, so this is rarely
needed in pipelines.

### Resolve a rejected `--api-version`

Agents that pin an older schema may see `AIM_INVALID_FLAG`:

```sh
aim --api-version 0.9 list
# error: unsupported api-version: 0.9. Supported: 1.0.
```

Check `aim spec --version` for the current supported version. The
deprecation policy lives in [`docs/schema-changelog.md`](../schema-changelog.md).

## Increase verbosity

```sh
aim -V list      # debug
aim -VV list     # trace
```

## Reach a human

1. Re-read this page and `aim --help`.
2. Search existing issues on GitHub.
3. Open a new issue with:
   - `aim --version`
   - OS and architecture
   - Output of `aim status --format json`
   - Steps to reproduce
   - Expected vs actual behavior

## Related

- [Commands reference](commands.md) — every flag and subcommand.
- [Agent reference](agent.md) — error envelope codes, schema pinning.
- [Schema changelog](../schema-changelog.md) — supported `--api-version` values.
