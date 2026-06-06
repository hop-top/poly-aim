# Architecture: aim

> AI model registry CLI — query models.dev with 12-factor agent-safe
> contracts. Multi-language SDKs for embedding the same registry in
> Python, TypeScript, Rust, and PHP.

## What this document covers

The shape of the system, the layering choices, and why each
significant boundary exists. Operational topics (release flow,
contributing) live in [RELEASING.md](../RELEASING.md) and
[CONTRIBUTING.md](../CONTRIBUTING.md). Per-factor design rationale
for the CLI agent surface lives in
[docs/12-factor-conformance.md](12-factor-conformance.md).

## Goals + non-goals

**Goals**:

- A small, fast Go binary that wraps [models.dev](https://models.dev)
  as a queryable catalog of AI models, capabilities, and pricing.
- Library APIs in five languages with byte-for-byte JSON wire parity,
  so the same `models.dev/api.json` decodes consistently in any of
  them.
- An agent-safe CLI surface that satisfies the 12-factor AI-CLI
  contract (capability introspection, structured I/O, corrective
  errors, dry-run, idempotency, state transparency, schema evolution).
- Cross-language test parity via shared JSON fixtures.

**Non-goals**:

- Hosting or proxying model inference. aim is a registry index, not
  a gateway.
- Authoring a competing data source. aim consumes models.dev; data
  freshness is downstream of that.
- Language-specific CLIs. The 12-factor agent surface lives only in
  the Go binary. Python, TypeScript, Rust, and PHP ship as libraries.
- Long-running services. aim is a CLI + library; no daemons, no HTTP
  servers.

## High-level shape

```text
                ┌──────────────────────────┐
                │       models.dev         │
                │     (upstream JSON)      │
                └────────────┬─────────────┘
                             │ HTTP GET
                             ▼
                ┌──────────────────────────┐
                │   ModelsDevSource +      │
                │   circuit breaker        │
                └────────────┬─────────────┘
                             │ map[string]*Provider
                             ▼
                ┌──────────────────────────┐
                │      XDG file cache      │
                │   (atomic write, lock)   │
                └────────────┬─────────────┘
                             │ provider/model maps
                             ▼
                ┌──────────────────────────┐
                │        Registry          │
                │  (filter, query, lazy)   │
                └────────────┬─────────────┘
                             │
       ┌─────────────────────┼──────────────────────┐
       ▼                     ▼                      ▼
  ┌─────────┐         ┌─────────────┐        ┌─────────────┐
  │ Go CLI  │         │ Go library  │        │  Other SDKs │
  │ (12fa)  │         │ (embedders) │        │ py/ts/rs/php│
  └─────────┘         └─────────────┘        └─────────────┘
```

## Repository layout

```text
aim/
├── cmd/aim/                  go binary entry point
│   ├── main.go               cli.New, command tree wiring
│   └── main_test.go
├── internal/
│   ├── cmd/                  per-subcommand RunE implementations
│   ├── errs/                 structured error envelope constructors
│   ├── status/               aim-specific status providers (cache, source, …)
│   ├── apiversion/           --api-version negotiation
│   └── conformance/          12-factor TestFactor* + report generator
├── aim.go                    canonical types: Modalities, Limits, Cost, Model, Provider, Filter, Source
├── cache.go                  XDG file cache (atomic write, lockfile, TTL, stale-on-error)
├── source.go                 HTTP fetcher + breaker around models.dev
├── source_breaker_test.go
├── source_test.go
├── query.go                  DSL parser, ExplainQuery (parser-only)
├── query_test.go
├── registry.go               Registry: lazy load + filter + sort
├── registry_test.go
├── e2e_test.go
├── py/                       hop-top-aim — Python SDK (httpx)
├── ts/                       @hop-top/aim — TypeScript SDK (fetch + node:test)
├── rs/                       hop-top-aim — Rust crate (reqwest + tokio)
├── php/                      hop-top/aim — Composer package (Guzzle)
├── testdata/
│   ├── query-vectors.json    cross-SDK parser truth set
│   ├── registry-vectors.json cross-SDK filter truth set
│   ├── api-fixture.json      shared catalog for registry tests
│   └── api-schema.json       wire format JSON Schema
├── docs/                     user-facing docs (this file + the manual)
├── scripts/                  promote-release.sh + verify-sdk-parity.sh
└── .github/                  release-please config + workflows
```

## Layered architecture

The Go side has four layers with strict boundaries.

### Layer 1 — wire types (`aim.go`)

Pure data definitions matching `models.dev/api.json` 1:1. No
behavior, no I/O. The single file is reflected by `serde`-equivalent
shapes in every other SDK.

The types are deliberately wide (every optional field present even
when empty) so JSON round-trips are stable. `Filter` is the only
type with tristate `*bool` fields — these distinguish "don't filter"
from "must match `true`/`false`". Cross-language equivalents:

| Go | Python | TypeScript | Rust | PHP |
|----|--------|------------|------|-----|
| `*bool` | `Optional[bool]` | `boolean \| undefined` | `Option<bool>` | `?bool` |

The `Source` interface is in the same file because it's the seam
between the library and any upstream data provider. Anything
implementing `Fetch(ctx) -> map[string]*Provider` is a valid
substitute for `ModelsDevSource`.

### Layer 2 — fetching + caching (`source.go`, `cache.go`)

`ModelsDevSource` is the only network-touching code in the project.
Behavior:

- 30-second HTTP timeout.
- 50 MB response cap.
- Wrapped in a 3-strike circuit breaker (`source_breaker_test.go`):
  three consecutive failures open the breaker for 30 seconds;
  half-open after that; one success resets.

`Cache` lives in the XDG cache directory and owns atomic writes,
lockfile-guarded refresh, ETag-aware refetch, and stale-on-error
fallback (network failure with a warm cache returns the warm data
without bubbling the error). The cache exposes a `Meta()` accessor
so the CLI can emit provenance (`source`, `fetched_at`, `method`,
`cached`, `cache_age`) on every read response without re-reading
disk metadata.

The library lazily builds the `Cache` on first use via `Registry.Cache()`.
Embedders that want to share a cache across registries pass one
explicitly via the registry constructor.

### Layer 3 — query + filter (`query.go`, `registry.go`)

`ParseQuery` is a small hand-rolled tokenizer + tag-applier with
nine recognized keys (`in`, `out`, `provider`, `family`, `tool_call`,
`reasoning`, `open_weights`, `structured_output`, `temperature`)
plus free-text. It accepts quoted strings, comma-separated values
for modalities, repeated keys (append semantics), and rejects
unknown keys + malformed booleans + bare colons. The same grammar
runs in every SDK against the same `testdata/query-vectors.json`.

`ExplainQuery` is a parser-only path: it returns the parsed
`Filter` plus the free-text token slice without touching the cache
or network. Wired to `aim query --explain` for agent pre-validation.

`Registry` is the consumer-facing API: `Models(ctx, filter)`,
`Providers(ctx)`, and accessor methods for the cache and source URL
used by the CLI's provenance envelope. Filtering is in-memory and
deterministic (alphabetical by `Provider.ID` then `Model.ID`); all
non-zero filter fields are ANDed; modality fields use subset
containment.

### Layer 4 — CLI surface (`cmd/aim/`, `internal/`)

The binary wires kit's `cli.Root` with five adopter leaves (`list`,
`show`, `providers`, `query`, `refresh`) plus two kit-shipped
reserved leaves (`spec`, `status`). Every leaf declares its
side-effect class, idempotency, output schema, examples, and
next-step hints via kit's contract annotations. The validator at
`cli.New` time rejects boot if anything is missing — there is no
way to ship a leaf that doesn't satisfy the agent contract.

**Internal packages**:

- `internal/cmd/` — one file per leaf. Each `*Cmd(root)` constructor
  returns a `*cobra.Command` already annotated.
- `internal/errs/` — error-envelope constructors. Every RunE returns
  `*output.Error`; kit's `WrapRunE` middleware renders it through
  `RenderError` so the wire format follows `--format`.
- `internal/status/` — `StatusProvider` implementations for cache,
  source, source-breaker, identity, paths, environment. Honors a
  strict env-var allowlist + sensitive-name redaction.
- `internal/apiversion/` — `--api-version` negotiation. Wraps every
  leaf's `RunE` so unsupported versions fail fast with a structured
  envelope, not a bare exit code.
- `internal/conformance/` — `TestFactor1`...`TestFactor12` plus
  `TestGenerateReport`. The report (`docs/12-factor-conformance.md`)
  is regenerated by running the test.

## Cross-SDK boundary

Python, TypeScript, Rust, and PHP each ship as a standalone library
package — no shared toolchain, no shared build, no imports across
language boundaries. The only coupling points:

1. **Wire format**: `models.dev/api.json` is the contract. Every SDK
   decodes the same JSON into structurally equivalent types.
2. **Query DSL**: every SDK accepts the same grammar against
   `testdata/query-vectors.json`. The Go binary's `aim query` is
   the canonical implementation.
3. **Filter semantics**: every SDK's `Registry.models(filter)`
   returns the same model list against `testdata/registry-vectors.json`
   given the same `testdata/api-fixture.json`.

The cross-SDK conformance gate is `scripts/verify-sdk-parity.sh`
(also `make parity`). It runs every SDK's test suite against the
shared fixtures and fails on the first mismatch.

There is no central conformance daemon — the gate is a script that
each contributor and CI runs. If a fixture grows a vector, every SDK
must absorb it before merge.

## Key design decisions

### Library-only SDKs

Python, TS, Rust, and PHP are libraries with no CLI surface. The
12-factor agent contract lives only in the Go binary. Rationale:

- Agents use `aim` directly; humans use language-native libraries.
- Duplicating the envelope/error/spec subcommand stack 5x would
  multiply maintenance without changing the agent experience.
- The CLI is the agent affordance; the SDKs are the embedding
  affordance. Different audiences, different surfaces.

### Multi-component release-please

Each SDK is a separate release-please component with its own
prerelease channel. Commits scoped to `py/` open a release PR for
`aim-py` only; commits anywhere outside `py/`, `ts/`, `rs/`, `php/`
open one for the Go binary (`aim`). Cross-SDK changes promote all
five together until the API stabilizes.

This avoids the "single repo, single version" trap where a Python
bugfix forces a Rust version bump.

### Single source of truth for types

The Go file `aim.go` is the canonical type definition. Other SDKs
mirror it. When the Go file gains a field, the parity gate
(`make parity` driven by `query-vectors.json` + `registry-vectors.json`)
catches missing wire decode in any other SDK before merge.

Field additions are always additive. Renames are forbidden during
the alpha channel; after stable, renames go through one minor's
deprecation window per the schema-changelog policy.

### Envelope discipline

Every CLI command emits the same envelope structure under
`--format json` or `--format yaml`: `data` (payload), `_meta`
(provenance: source, fetched_at, method, cached, cache_age), and
`warnings` (deprecations) when applicable. This is provided by
kit's `output.Render` with `WithProvenance(meta)` — aim does not
ship its own envelope library.

Errors use the same structural discipline: `code`, `message`,
`cause`, `suggested_fix`, `alternatives`, `exit_code`. Every error
constructor lives in `internal/errs/`; kit's `WrapRunE` routes
returned errors through the wire-format renderer.

### Tristate filters

`Filter.ToolCall`, `Filter.Reasoning`, `Filter.OpenWeights`,
`Filter.StructuredOutput`, `Filter.Temperature` are tristate
pointers. `nil` means "don't filter" (the field is absent from the
query); `&true` / `&false` means "must match". This is deliberately
not the same as a default-value bool — agents need to distinguish
"please match models that don't support tool-calling" from "I'm
not filtering on tool-calling at all". The DSL parser respects this:
`tool_call:true` produces `&true`; absence produces `nil`.

### Idempotency by default

Every adopter leaf declares `kit/idempotent=yes`. The four read
leaves (`list`, `show`, `providers`, `query`) are pure functions
of the cached state. `refresh` is idempotent within the TTL window
and naturally idempotent under concurrent invocation (lockfile +
atomic rename). Agents can retry on transport failure without
fearing duplicate side effects.

### XDG-only state

aim has no config file. State is the cache directory (under
`$XDG_CACHE_HOME` or platform default) plus what the binary's
own flags pass at invocation. This keeps `aim status --format json`
fully self-describing — the output is the complete state, no
external file to check.

### Conformance as code

The 12-factor contract is enforced by `internal/conformance/`
TestFactor* tests, not by a checklist. Every factor has a test
that fails if the implementation drifts. The matrix in
`docs/12-factor-conformance.md` is auto-generated from the test
results, not hand-curated.

### Human-vs-agent help surfaces

aim ships two help surfaces deliberately. `aim --help` (and
`aim help <leaf>`) is cobra's built-in human-readable surface —
prose summary, flag table, examples. `aim spec --format json` is
the agent-facing capability manifest (clispec/v1) — structured
schema covering every leaf's flags, side-effect class,
idempotency, output schema, and next-step hints. The two surfaces
never disagree on what aim does, but only `aim spec` is
contract-stable: cobra's `--help` layout can shift with
upstream changes, while clispec is versioned via `--api-version`.
Agents should always introspect via `aim spec`; humans get both.

### Per-invocation circuit breaker

The source-breaker state is process-local by design. aim is a CLI
binary, not a daemon — each invocation owns its own `*Registry`
and therefore its own breaker. Sharing breaker state across
processes would require a coordination mechanism (lockfile,
shared memory, Redis, system service) that buys little for a
fast-exit CLI: each invocation independently discovers
upstream-down within three failed fetches and trips locally.
Long-running Go-library embedders DO get within-process
amortization — calls to `Registry.Models` reuse the same breaker
for the lifetime of the `*Registry`, so a single embedder
detects upstream-down once and skips retries until the breaker
half-opens. If a future embedding context needs cross-process
breaker state (e.g., a long-lived service wrapping aim as a
library), the coordination is the embedder's responsibility, not
aim's.

## Extension points

| To add… | Touch… |
|---------|--------|
| A new wire-format field | `aim.go` (Model or Filter), update parser if applicable, update parity fixtures, all SDKs follow |
| A new query DSL key | `query.go` `knownTagKeys` + `applyTag`, update fixture, all SDKs follow |
| A new CLI leaf | `internal/cmd/<name>.go`, register in `cmd/aim/main.go`, annotate via `cli.Set*`, add factor tests in `internal/conformance/` if needed |
| A new status section | New `StatusProvider` in `internal/status/`, register in `Register()` |
| A new error code | New constructor in `internal/errs/`, document in `docs/manual/commands.md` error catalog |
| A new SDK language | Mirror the four-file shape (types, query, source, registry), drive shared fixtures, add to release-please config + `make parity` script |

## Open issues

- **Upstream-dependent — track but don't act.** `cmd/aim/main.go`
  hand-walks the command tree at boot to wrap every leaf's `RunE`
  with `--api-version` validation. Kit (`v0.4.0-alpha.6`) does not
  ship native persistent-flag-rejection middleware that routes
  through the structured error envelope, so aim owns the walker.
  If a future kit release adds a `cli.WithFlagValidator(name, fn)`
  style hook that calls `output.RenderError` on rejection, the
  walker can collapse to a single registration call and
  `installAPIVersionGuard` / `walkCommands` can be deleted. Track
  at `hop.top/kit` — no aim-side work until then.
