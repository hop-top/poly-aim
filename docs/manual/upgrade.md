# Upgrade aim

## Use this page when

- Moving an installed `aim` binary to a newer version.
- Pinning to a specific tag for reproducibility.
- Rolling back after a regression.

## Result

After this guide, `aim --version` reports the version you intended,
and your local cache continues to work without re-fetching.

## Before you begin

```sh
aim --version            # current binary
aim spec --version       # schema version (what agents pin)
```

## Quick version

```sh
go install hop.top/aim/cmd/aim@latest
aim --version
```

## Upgrade paths

### Go binary (canonical)

```sh
go install hop.top/aim/cmd/aim@latest
```

### From source

```sh
cd <your aim clone>
git pull origin main
make build
./bin/aim --version
```

### Language SDKs

Until the first stable release, each SDK ships from source. Pull the
repo and rebuild. Distribution packages (PyPI / npm / crates.io /
Packagist) are planned for `0.1.0` stable.

## Pin a specific version

```sh
go install hop.top/aim/cmd/aim@v0.1.0-alpha.N
```

Tag format:

- Go binary: `aim/v<version>`
- Each SDK: `aim-<lang>/v<version>`

## Rollback

If a release introduces a regression, install a previous tag:

```sh
go install hop.top/aim/cmd/aim@v0.1.0-alpha.<N-1>
```

Your XDG cache survives any binary swap — no re-refresh needed unless
the schema bumped.

## Breaking changes during alpha

aim ships during prerelease as `0.1.0-alpha.N`. Breaking changes are
expected within the alpha channel. Each component (Go, py, ts, rs,
php) advances through `alpha -> beta -> rc -> release` independently
via release-please.

The CLI schema follows a separate policy — see
[`docs/schema-changelog.md`](../schema-changelog.md) for envelope,
error envelope, and capability-manifest evolution rules. Agents pin
to a specific `--api-version` for stability across binary bumps.

## Verify

```sh
aim --version
aim spec --version
aim list --format json | jq '.provenance.fetched_at'
```

All three should report sane values. If `list` fails, see
[troubleshooting.md](troubleshooting.md).

## Related

- [Schema changelog](../schema-changelog.md) — `--api-version` history.
- [Release process](../../RELEASING.md) — how versions are cut.
- [Troubleshooting](troubleshooting.md).
