# Install aim

Choose an install method and verify it works.

## Use this page when

- Installing the canonical Go binary.
- Building from source against the current `main`.
- Picking one of the language SDKs (until distribution packages land).

## Result

After this guide, `aim --version` prints a non-empty version string.

## Before you begin

- Go 1.26+ for `go install` (canonical path).
- Or: a release binary for your platform (planned for first stable).

## Quick version

```sh
go install hop.top/aim/cmd/aim@latest
aim --version
```

## Install paths

### Go binary (recommended)

```sh
go install hop.top/aim/cmd/aim@latest
```

### From source

```sh
git clone https://github.com/hop-top/aim
cd aim
make build              # produces ./bin/aim
./bin/aim --version
```

### Language SDKs

Until the first stable release, SDK consumers (Python, TypeScript,
Rust, PHP) ship from this repo — pull the repo and use the per-SDK
sources.

- `py/` — `hop-top-aim` (Python, `uv`)
- `ts/` — `@hop-top/aim` (TypeScript, native node test runner)
- [`rs/`](../../rs/README.md) — `hop-top-aim` (Rust crate)
- [`php/`](../../php/README.md) — `hop-top/aim` (Composer package)

## Verify

```sh
aim --version
```

Then refresh the cache and run your first query — see
[get started](getting-started.md).

## System requirements

| Component | Min version | Notes |
|-----------|-------------|-------|
| Go        | 1.26        | for building the canonical binary |
| Node      | 22+         | for TS SDK; needs `--experimental-strip-types` |
| Python    | 3.11        | for py SDK; uses `uv` for env management |
| Rust      | 1.78        | for rs SDK |
| PHP       | 8.2         | for php SDK |
| Docker    | any         | optional, only for `make dev-up` devcontainer |

## Cache location

| Platform | Path |
|----------|------|
| macOS    | `~/Library/Caches/hop/aim/` |
| Linux    | `~/.cache/hop/aim/` |
| Windows  | `%LOCALAPPDATA%\hop\aim\` |

`$XDG_CACHE_HOME` overrides on every platform. To inspect the resolved
path on your machine:

```sh
aim status --format json | jq '.sections[] | select(.title == "cache")'
```

## Related

- [Get started](getting-started.md) — first refresh + query.
- [Configuration](configuration.md) — env vars and overrides.
- [Upgrade](upgrade.md) — moving between versions.
