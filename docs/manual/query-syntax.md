# Query syntax reference

The query DSL used by `aim query <expr>`, `aim list <expr>` (deprecated
positional), and the cross-language `ParseQuery` / `parse_query`
functions in every SDK.

## Use this page when

- Writing a query expression by hand for `aim query`.
- Generating expressions from an agent or other program.
- Debugging a `INVALID_QUERY` error.

## Quick version

```text
provider:openai tool_call:true "gpt-4o"
in:image,text reasoning:false
```

Tags AND together. Modality tags (`in`, `out`) accept CSV lists. Bare
tokens and `"quoted strings"` are case-insensitive substring matches
against model ID and name.

## Tags

| Tag | Type | Example | Match |
|-----|------|---------|-------|
| `in` | string (CSV) | `in:image,text` | input modality; subset; repeatable |
| `out` | string (CSV) | `out:text` | output modality; subset; repeatable |
| `provider` | string | `provider:openai` | exact, case-sensitive |
| `family` | string | `family:gpt4` | exact, case-sensitive |
| `tool_call` | bool | `tool_call:true` | `true` or `false` only |
| `reasoning` | bool | `reasoning:false` | `true` or `false` only |
| `open_weights` | bool | `open_weights:true` | `true` or `false` only |
| `structured_output` | bool | `structured_output:true` | `true` or `false` only |
| `temperature` | bool | `temperature:true` | `true` or `false` only |

## Free-text

Bare tokens (no colon) and `"quoted strings"` are case-insensitive
substring matches against `Model.ID` and `Model.Name`.

```text
gpt-4o                          # matches id/name containing "gpt-4o"
"model:special"                 # colon inside quotes is literal
provider:openai "gpt-4o"        # tag + free-text combined
```

## Multi-value / repeated keys

```text
in:image,video                  # comma-separated list
in:image in:video               # same — repeated keys append
```

## Verify

Use `--explain` to confirm a query parses as expected without running it:

```sh
aim query --explain 'provider:openai tool_call:true' --format json
```

Returns the parsed AST (`expr_input`, `filter`, `free_text`). No cache
read, no network call.

## Error cases

| Input | Error |
|-------|-------|
| `tool_call:yes` | invalid bool — must be `true` or `false` |
| `tool_call:1` | invalid bool |
| `unknown_key:val` | unknown tag key |
| `:` | bare colon |
| `"unterminated` | unterminated quoted string |
| `key:` or `:val` | empty key or value |
| `cost:...` | unknown — no query syntax yet (use programmatic `Filter` + post-filter on `Model.Cost`) |
| `attachment:...` | unknown — no query syntax yet (use programmatic `Filter` + post-filter on `Model.Attachment`) |

All cases surface through the `INVALID_QUERY` envelope. The
`suggested_fix` field names the offending token.

## How matching works

- Modality tags (`in`, `out`) use **subset containment**:
  `Filter.Input ⊆ Model.Modalities.Input`.
- Tags AND together.
- Free-text tokens AND with tag filters.
- Free-text tokens AND with each other.
- An empty expression matches every model.

## Related

- [Commands reference](commands.md) — `aim query`, `aim list`, `aim show`.
- [Library reference](library.md) — programmatic `Filter` struct (lets
  you express cost and attachment predicates not yet covered by the DSL).
