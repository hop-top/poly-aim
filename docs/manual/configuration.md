# Configuration reference

aim is config-free by design. There is no `aim.yaml`, no
`~/.config/aim/` file. State lives in two places: the XDG cache
(managed by aim) and environment variables (honored at runtime).

## Use this page when

- Inspecting the resolved configuration on a machine.
- Overriding the cache location via XDG.
- Suppressing color, hints, or sensitive values in output.

## Inspect resolved state

```sh
aim status --format json
```

The single source of truth — it reports the active cache dir, TTL,
source URL, environment, and identity.

## Environment variables

| Variable | Effect |
|----------|--------|
| `XDG_CACHE_HOME` | Override cache root (default: platform-specific) |
| `XDG_CONFIG_HOME` | Override config root (currently unused by aim) |
| `XDG_DATA_HOME` | Override data root (currently unused by aim) |
| `NO_COLOR` | Disable ANSI color output |
| `AIM_*` | Reserved for future aim-owned flags |

Sensitive name patterns are redacted from `aim status` output:
`TOKEN`, `SECRET`, `KEY`, `PASSWORD` (case-insensitive substring).
Use `--show-sensitive` to override.

## CLI flags

Configuration that would normally live in a config file is expressed
as flags at invocation time.

| Flag | Purpose |
|------|---------|
| `--format <table\|json\|yaml>` | Output format |
| `-V` / `--verbose` | Log verbosity (repeatable) |
| `--no-color` | Disable color |
| `--no-hints` | Suppress next-step hints |
| `--api-version <X.Y>` | Negotiate schema version |
| `--dry-run` | Preview side effects (refresh only) |

## Cache TTL

Default TTL is 24 hours. To check active TTL and remaining time:

```sh
aim status --format json | jq '.sections[] | select(.title == "cache")'
```

The cache library accepts custom TTL when used programmatically — see
[library.md](library.md). The CLI uses the default.

## Precedence

CLI flags only. No file-based config, no environment hierarchy beyond
the XDG path overrides above. State is whatever the cache + source URL
report.

## For agents

`aim spec --format json` returns the full capability manifest
including which flags each command supports and their valid values.
See [agent.md](agent.md) for the agent-driving workflow.

## Related

- [Get started](getting-started.md) — initial cache refresh.
- [Library reference](library.md) — programmatic cache options.
- [Troubleshooting](troubleshooting.md) — when state is wrong.
