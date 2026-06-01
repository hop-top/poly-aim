# Commands reference

Every aim subcommand, its flags, side effects, and the envelope it
returns. This is the canonical CLI reference — the README points here.

## Use this page when

- Looking up a flag.
- Confirming which commands are write-effecting vs read-only.
- Picking the right verb (`list` vs `query` vs `show`).

## Global flags

aim inherits kit's persistent flags. Most are hidden under `--help-all`.

| Flag | Purpose |
|------|---------|
| `--format <table\|json\|yaml>` | Output format. Default: `table` on TTY, `json` otherwise |
| `--no-color` | Disable ANSI color |
| `--no-hints` | Suppress next-step hint output |
| `--quiet` | Suppress non-essential output |
| `-V` / `--verbose` | Increase log verbosity (repeatable) |
| `--api-version <X.Y>` | Negotiate schema version (rejects unsupported) |
| `--dry-run` | Preview side effects without applying (where supported) |

Run `aim --help` for the visible set, `aim --help-all` for everything.

**Agents**: use `aim spec --format json` for the capability schema —
`--help` is for humans. The two surfaces never disagree, but only
`aim spec` is contract-stable. See [agent.md](agent.md).

## Reads

### List models — `aim list`

```sh
aim list                                      # all models, table
aim list --provider openai                    # flag-built filter
aim list --tool-call --reasoning --format json
```

Read-only, idempotent. Returns the standard envelope with provenance.

> [!NOTE]
> **Positional arg is deprecated.** `aim list <expr>` still works but
> emits a deprecation warning in `envelope.warnings[]`. Use
> `aim query <expr>` for DSL syntax or the equivalent flag
> (e.g. `--family`) for flag filters.

| Flag | Type | Notes |
|------|------|-------|
| `--input` | string slice | input modalities |
| `--output` | string slice | output modalities |
| `--provider` | string | provider ID |
| `--family` | string | model family |
| `--tool-call` | bool | require tool-call support |
| `--reasoning` | bool | require reasoning support |
| `--open-weights` | bool | require open weights |
| `--format` | string | `table`, `json`, `yaml` |

### Show one model — `aim show <provider> <model>`

```sh
aim show openai gpt-4o
aim show --provider openai --model gpt-4o
aim show anthropic claude-3-5-sonnet --format json
```

Read-only, idempotent. Accepts positional args or the `--provider`/
`--model` aliases. When both forms are supplied the positional values
win and a warning surfaces.

| Flag | Type | Notes |
|------|------|-------|
| `--provider` | string | provider ID (alias for first positional) |
| `--model` | string | model ID (alias for second positional) |
| `--format` | string | `table`, `json`, `yaml` |

### List providers — `aim providers`

```sh
aim providers
aim providers --format yaml
```

Read-only, idempotent. Returns provider ID, name, model count.

### Run a query — `aim query <expr>`

```sh
aim query 'in:image tool_call:true'
aim query 'provider:openai "gpt"'
aim query --explain 'provider:openai tool_call:true' --format json
```

Read-only, idempotent. Full grammar in [query-syntax.md](query-syntax.md).

`--explain` parses the expression and emits the parsed AST without
running the query — no cache read, no network call. Useful for
agents that want to validate DSL shape before executing. Invalid
expressions still flow through the `INVALID_QUERY` envelope.

## Writes

### Refresh the cache — `aim refresh`

```sh
aim refresh             # skip if within TTL
aim refresh --force     # bypass TTL; always re-fetch
aim refresh --dry-run   # report what would happen; no fetch, no writes
```

Write-local (XDG cache dir only), idempotent.

`--dry-run` returns a `RefreshPreview` payload describing the intended
action — `status` (`would_refresh` or `would_skip`), `reason`,
`would_fetch_url`, `would_write_paths`, current ETag, last fetch, and
TTL remaining. The dry-run path makes zero network calls and never
writes; it exits 0 regardless of cache state. Combine with `--force`
to preview a forced re-fetch.

## Introspection

### Capability manifest — `aim spec`

```sh
aim spec --format json
aim spec --version       # just {"schema_version":"1.0"}
```

Read-only, idempotent. The clispec/v1 manifest agents use to discover
the full CLI surface. See [agent.md](agent.md) for the full agent
workflow.

### Runtime state — `aim status`

```sh
aim status --format json
aim status --show-sensitive    # include redacted env values
```

Read-only, idempotent. Returns kit-shipped sections (`profile`, `env`,
`workspace`, `auth`, `effective-config`, `kit-annotations`) and
aim-shipped sections (`cache`, `source`, `source-breaker`, `identity`,
`paths`, `environment`).

Honors the env-key allowlist documented in [12-factor conformance](../12-factor-conformance.md).

## Side effects + idempotency

| Command | Side effect | Idempotent |
|---------|-------------|-----------|
| `aim spec` | none | yes |
| `aim status` | none | yes |
| `aim list` | read | yes |
| `aim show` | read | yes |
| `aim providers` | read | yes |
| `aim query` | read | yes |
| `aim refresh` | write (cache dir) | yes |

aim never writes outside its XDG cache directory. Per-leaf annotations
are exposed in `aim spec --format json`.

## Error envelope

Every error returns a structured payload on stderr:

```json
{
  "code": "NOT_FOUND",
  "message": "provider not found: nope",
  "suggested_fix": "run `aim providers` to list valid provider IDs",
  "alternatives": ["aim providers", "aim list", "aim refresh"],
  "exit_code": 64
}
```

Stable code catalog: see [agent.md](agent.md#error-envelope). Source
of truth: [`internal/errs/errs.go`](../../internal/errs/errs.go). Exit
codes follow `sysexits.h`.

## Related

- [Get started](getting-started.md) — first-run walkthrough.
- [Query syntax](query-syntax.md) — DSL grammar.
- [Drive aim from an agent](agent.md) — schema pinning, error codes.
- [Embed aim as a library](library.md) — programmatic API.
- [Configuration](configuration.md) — env vars and flag overrides.
- [Troubleshooting](troubleshooting.md) — recovery for common failures.
