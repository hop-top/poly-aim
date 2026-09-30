# hop.top/aim — Go library + CLI

AI model registry client backed by [models.dev](https://models.dev).
Canonical implementation: the agent-safe `aim` CLI and the Go library
every other SDK mirrors.

Module: `hop.top/aim` · Go 1.26+ · MIT.

## Install

```sh
go install hop.top/aim/cmd/aim@latest   # CLI
go get hop.top/aim                      # library
```

## Quickstart

```sh
aim refresh
aim list --provider openai --tool-call
```

```go
import "hop.top/aim"

reg := aim.NewRegistry()
models, err := reg.Models(ctx, aim.Filter{Input: []string{"image"}})
```

## Provider facts

Aliases, credential env vars vs settings, local (loopback) or not,
protocol and default base URL, derived from the catalog:

```go
p, ok, err := reg.Provider(ctx, "gemini") // id or alias
if err != nil || !ok { ... }

p.ID          // "google"
p.KeyVars()   // ["GOOGLE_API_KEY", "GOOGLE_GENERATIVE_AI_API_KEY", "GEMINI_API_KEY"]
p.IsLocal()   // false
p.Protocol()  // "google"

aim.CanonicalProviderID("together") // "togetherai"; no catalog needed
```

Rules for each fact: [library.md](https://github.com/hop-top/poly-aim/blob/main/docs/manual/library.md#provider-facts).

## Docs

Source, issues and the full manual live in the monorepo,
[hop-top/poly-aim](https://github.com/hop-top/poly-aim):

- [Get started](https://github.com/hop-top/poly-aim/blob/main/docs/manual/getting-started.md)
- [Embed aim as a library](https://github.com/hop-top/poly-aim/blob/main/docs/manual/library.md)
- [Commands reference](https://github.com/hop-top/poly-aim/blob/main/docs/manual/commands.md)

## License

MIT
