# Embed aim as a library

Programmatic access to the models.dev catalog from your own code. Same
cache, same query semantics as the CLI — no subprocess.

## Use this page when

- Importing `hop.top/aim` into a Go program.
- Picking the equivalent SDK in Python, TypeScript, Rust, or PHP.
- Driving a custom registry source (private catalog, static file).
- Tuning cache TTL, location, or max payload size.

## Result

After this guide, you will:

- Construct a `Registry` with default or custom options.
- Query models programmatically via `Filter` or the DSL.
- Plug in a custom `Source` to feed from an internal registry.

## Quick version (Go)

```go
import "hop.top/aim"

reg := aim.NewRegistry()
models, err := reg.Models(ctx, aim.Filter{Input: []string{"image"}})
```

## Install

```sh
go get hop.top/aim
```

Other languages: see the per-SDK READMEs.

| Language | Path | Distribution |
|----------|------|--------------|
| Go (canonical) | `.` | `go get hop.top/aim` |
| Python | `py/` | `pip install hop-top-aim` |
| TypeScript | `ts/` | `npm install @hop-top/aim` |
| Rust | `rs/` | `cargo add hop-top-aim` |
| PHP | `php/` | `composer require hop-top/aim` |

The cross-SDK parity matrix lives in [`docs/sdk-parity.md`](../sdk-parity.md).

## Filter struct

```go
f := aim.Filter{
    Input:    []string{"image"},
    Provider: "anthropic",
}
trueVal := true
f.ToolCall = &trueVal               // tristate: nil = no filter

models, err := reg.Models(ctx, f)
```

All non-zero fields AND together. Modality fields use **subset
containment**: `Filter.Input ⊆ Model.Modalities.Input`.

### Tristate fields

`*bool` — `nil` skips the filter; `&true` / `&false` requires exact match.

| Field | Filters on |
|-------|------------|
| `ToolCall` | tool-call support |
| `Reasoning` | reasoning support |
| `OpenWeights` | open weights |
| `StructuredOutput` | structured output |
| `Temperature` | temperature parameter |

The same convention exists in every SDK — `Optional[bool]` (Python),
`boolean | undefined` (TypeScript), `Option<bool>` (Rust), `?bool` (PHP).

## Cost field

`Model.Cost` mirrors the models.dev `cost` object. Values are USD per
1M tokens.

```go
if m.Cost != nil && m.Cost.Input > 0 {
    // priced model
}
```

`Cost` is `*Cost`:

- `nil` — upstream omitted `cost` entirely (typical for open-weight
  / local models).
- non-nil with zero fields — `cost` was explicitly present but unpriced.

No `cost:` query tag exists yet. Filter via `Registry.Models(ctx, Filter{...})`
and post-filter on `Model.Cost` directly.

## Query DSL from code

The same DSL the CLI uses:

```go
f, freeText, err := aim.ParseQuery("provider:openai tool_call:true gpt-4o")
if err != nil { ... }
models, err := reg.Query(ctx, f, freeText)
```

Or compose programmatically — full DSL grammar in
[query-syntax.md](query-syntax.md).

## Provider facts

Facts about a provider, derived from its catalog entry. No network call
beyond loading the catalog; alias resolution needs no catalog at all.

```go
p, ok, err := reg.Provider(ctx, "gemini") // id or alias
if err != nil || !ok { ... }

p.ID          // "google" — canonical models.dev id
p.Aliases()   // ["gemini"]
p.KeyVars()   // ["GOOGLE_API_KEY", "GOOGLE_GENERATIVE_AI_API_KEY", "GEMINI_API_KEY"]
p.Settings()  // [] — non-credential env vars (region, project, ...)
p.IsLocal()   // false — true when p.API is on a loopback host
p.Protocol()  // "google"
p.API         // "" — default base URL; empty when the SDK package knows it
```

| Fact | Rule |
|------|------|
| `Aliases()` | curated overlay: `fireworks` → `fireworks-ai`, `gemini` → `google`, `together` → `togetherai` |
| `KeyVars()` | `Env` names ending `_KEY` (so also `_API_KEY`), `_APIKEY`, `_PAT` or `_TOKEN`, catalog order (first = preferred) |
| `Settings()` | every other `Env` name, catalog order |
| `IsLocal()` | `API` host is `localhost`, `*.localhost`, or a loopback IP (`127.0.0.0/8`, `::1`); key optional |
| `Protocol()` | from `NPM`: `@ai-sdk/<name>` → `<name>` for listed packages, `@openrouter/ai-sdk-provider` → `openrouter`; unlisted → `""` |
| `API` | the catalog `api` field, verbatim; may hold `${VAR}` placeholders naming a setting |

Without a catalog (offline, cold cache), resolve names statically:

```go
aim.CanonicalProviderID("together") // "togetherai"; unknown names unchanged
aim.ProviderAliases("google")       // ["gemini"]; accepts an alias too
```

`Registry.Provider` prefers a catalog id over an alias of the same
name. Multi-variable providers (Bedrock, Azure, Vertex) are classified
by the same rule; interpreting their settings is left to the caller.
For example Bedrock's `AWS_BEARER_TOKEN_BEDROCK` and `AWS_ACCESS_KEY_ID`
are settings (no key suffix), `AWS_SECRET_ACCESS_KEY` is a key var, and
Vertex's `GOOGLE_APPLICATION_CREDENTIALS` (a file path) is a setting.

Conformance fixtures shared by every SDK:
`testdata/provider-fixture.json` (input catalog),
`testdata/provider-facts-vectors.json`,
`testdata/provider-lookup-vectors.json`,
`testdata/provider-protocol-vectors.json`.

## Custom source

Implement `Source` to feed from an internal registry or static file:

```go
type MySource struct{}

func (s *MySource) Fetch(ctx context.Context) (map[string]*aim.Provider, error) {
    // return map[providerID]*aim.Provider
    // map key MUST equal Provider.ID
    // Model.Provider is backfilled from the parent key automatically
}

reg := aim.NewRegistry(aim.WithSource(&MySource{}))
```

Optional ETag optimisation:

```go
func (s *MySource) FetchWithETag(ctx context.Context, etag string) (
    map[string]*aim.Provider, string, error,
) {
    // return (nil, etag, nil) on 304 Not Modified
}
```

## Cache options

```go
reg := aim.NewRegistry(
    aim.WithCacheOpts(
        aim.WithCacheDir("/tmp/aim"),
        aim.WithTTL(time.Hour),
        aim.WithMaxSize(100 * 1024 * 1024),
    ),
)
```

| Option | Default | Purpose |
|--------|---------|---------|
| `WithTTL(d)` | 24h | cache freshness window |
| `WithCacheDir(path)` | XDG | cache directory |
| `WithMaxSize(n)` | 50 MB | refuse payloads larger than this |

### Cache layout

```text
<cache-dir>/
  models-dev.json   — cached api.json payload
  meta.json         — {last_fetch, etag, ttl_seconds}
  .lock             — sentinel; covers fetch+write cycle
```

### Behaviour

- **Stale-on-error**: network failure with warm cache returns stale
  data, no error.
- **ETag**: `If-None-Match` sent on every refresh; 304 bumps
  `last_fetch` without re-downloading.
- **Force refresh**: `reg.Refresh(ctx)`.

## Verify

```go
models, err := reg.Models(ctx, aim.Filter{})
if err != nil { return err }
if len(models) == 0 {
    // empty registry — likely fresh install
    _ = reg.Refresh(ctx)
}
```

## Related

- [Query syntax](query-syntax.md) — DSL grammar.
- [Configuration](configuration.md) — XDG cache paths, env vars.
- [Cross-SDK parity matrix](../sdk-parity.md) — feature equivalence
  across Go, Python, TypeScript, Rust, PHP.
- [Architecture](../ARCHITECTURE.md) — how the cache, source, and
  registry layers fit together.
