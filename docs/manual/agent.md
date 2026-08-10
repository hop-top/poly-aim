# Drive aim from an agent

aim is built so a dispatching agent can read its capability schema
once and then drive every subcommand safely — no human help text, no
guesswork about side effects, idempotency, or error shape.

## Use this page when

- Wiring aim into an LLM agent's tool list.
- Building a script or orchestrator that calls aim.
- Pinning a stable schema version for production.

## Result

After this guide, your agent will:

- Discover aim's full surface via `aim spec --format json`.
- Pin to a specific schema with `--api-version`.
- Parse structured envelopes and error envelopes without screen-scraping.
- Know which commands are safe to retry and which are write-effecting.

## Quick version

```sh
# 1. Discover.
aim spec --format json > aim-spec.json

# 2. Pin schema version.
aim --api-version 1.0 list --format json

# 3. Preview any write.
aim refresh --dry-run --format json
```

## Capability discovery

```sh
aim spec --format json          # full manifest
aim spec --version              # tiny probe — just {"schema_version":"1.0"}
```

The manifest follows the **clispec/v1** schema. For each leaf command
it exposes:

- `output_schema` — JSON Schema of the success envelope's `data` field.
- `examples` — runnable invocations.
- `next_steps` — typical follow-up commands.
- `side_effect` — `none`, `read`, or `write`.
- `idempotency` — `yes` or `no`.

Use `output_schema` to validate parsed responses. Use `side_effect` to
decide whether a command can run unattended.

## Stable schema pinning

Pin `--api-version` so future schema bumps don't break your agent:

```sh
aim --api-version 1.0 list --format json
```

Lower or unknown versions return an `AIM_INVALID_FLAG` envelope with
`alternatives` listing `aim spec` so the agent can re-discover.

Supported versions live in [`docs/schema-changelog.md`](../schema-changelog.md).
That file is the canonical history of every shipped schema version,
deprecations, and removal dates.

## Success envelope

Every leaf returns the same shape on `--format json`:

```json
{
  "data": { ... },
  "warnings": [],
  "provenance": {
    "source": "https://models.dev/api.json",
    "fetched_at": "2026-06-01T12:00:00Z",
    "method": "http_get_cached"
  }
}
```

`warnings[]` carries non-fatal notices (e.g. deprecated positional
args). `provenance` is present on every read so downstream systems
know where data came from.

## Error envelope

Every error returns the same shape on stderr:

```json
{
  "code": "NOT_FOUND",
  "message": "provider not found: nope",
  "cause": "nope",
  "suggested_fix": "run `aim providers` to list valid provider IDs",
  "alternatives": ["aim providers", "aim list"],
  "exit_code": 3
}
```

Stable error codes (see [`internal/errs/errs.go`](../../internal/errs/errs.go)
for the full catalog):

| Code | Exit | Meaning |
|------|------|---------|
| `NOT_FOUND` | 3 | Provider or model not in the cache |
| `INVALID_QUERY` | 2 | Query DSL syntax error |
| `AIM_INVALID_FLAG` | 2 | Unsupported `--api-version`, invalid flag value |
| `AIM_NETWORK` | 6 | Transport-level failure on source fetch |
| `AIM_CACHE_CORRUPT` | 1 | Cache file present but unparseable |
| `AIM_SOURCE_UNAVAILABLE` | 6 | Source breaker open after repeated failures |
| `AIM_CACHE_LOCKED` | 4 | Concurrent writer holds the cache lock |

The process exit code always matches the envelope's `exit_code`. Codes
follow the shared taxonomy — 0 success, 1 general, 2 usage, 3 not-found,
4 conflict, 5 permission, 6 transient/retryable, 64 rate-limited — so
exit 6 means "back off and retry the same invocation", while 1–4 mean
"retrying unchanged will fail again". Per-command exit classes are
published in `aim spec --format json` under `commands[].exit_codes`.

`suggested_fix` and `alternatives` are deliberately **runnable** — an
agent can shell out to the suggestion without further parsing.

## Preview side effects

Any write-effecting command supports `--dry-run`:

```sh
aim refresh --dry-run --format json
```

Returns a `RefreshPreview` payload (`status`, `reason`, `would_fetch_url`,
`would_write_paths`, current ETag, TTL remaining) with **zero network
calls** and **zero writes**.

Parse-only on the query DSL:

```sh
aim query --explain 'provider:openai' --format json
```

Returns the parsed AST without reading the cache.

## Runtime state

```sh
aim status --format json
```

Returns kit-shipped sections (`profile`, `env`, `workspace`, `auth`,
`effective_config`, `kit_annotations`) plus aim-shipped sections
(`cache`, `source`, `source_breaker`, `identity`, `paths`,
`environment`). Use this to inspect cache age, current source URL,
and circuit-breaker state before deciding whether to refresh.

## Suppress hints

aim emits next-step hints to humans on a TTY. Suppress for scripted use:

```sh
aim list --no-hints --format json
```

Hints are also auto-suppressed when stdout is not a terminal, so the
flag is rarely needed in pipelines.

## Side effects + idempotency

| Command | Side effect | Idempotent | Notes |
|---------|-------------|-----------|-------|
| `aim spec` | none | yes | pure read of compile-time data |
| `aim status` | none | yes | reads runtime state only |
| `aim list` | read | yes | cache read |
| `aim show` | read | yes | cache read |
| `aim providers` | read | yes | cache read |
| `aim query` | read | yes | cache read |
| `aim refresh` | write (cache dir) | yes | re-running within TTL is a no-op |

aim never writes outside its XDG cache directory. The full per-leaf
annotations are exposed in `aim spec --format json`.

## Verify the contract

```sh
go test -run TestGenerateReport ./internal/conformance/...
```

Regenerates [`docs/12-factor-conformance.md`](../12-factor-conformance.md)
— aim's executable proof that all 12 factors pass. The same test
suite runs in CI on every change.

## Related

- [Commands reference](commands.md) — full CLI surface.
- [Query syntax](query-syntax.md) — DSL grammar.
- [12-factor conformance report](../12-factor-conformance.md).
- [Schema changelog](../schema-changelog.md) — supported `--api-version` values.
- [12-factor AI-CLI spec](https://github.com/hop-top/12-factor-ai-cli-apps).
