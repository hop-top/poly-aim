---
title: aim schema changelog
status: living
owner: aim
---

# aim schema changelog

Canonical history of every MAJOR.MINOR bump aim has shipped. Agents
that pin `--api-version` consult this file (or `aim spec --version`)
to learn what shape they will receive.

Versioning rules and deprecation policy live in
`.tlc/tracks/aim-12-factor-conformance/spec.md` under
`## Schema versioning policy`. This file is the rolling log; that file
is the contract.

## 1.0.0 (initial)

Pinned as `aim/v1` on every leaf, `1.0` on the top-level capability
manifest, and accepted by `--api-version`.

- Capability schema: `clispec/v1`, top-level `schema_version` `1.0`
  (emitted by `aim spec`; mounted via
  `cli.RegisterSpecCommand(root, apiversion.Current)`).
- Envelope wire shape: `{data, _meta, warnings, hints}` — `data` and
  `_meta` populated by every leaf; `warnings`/`hints` reserved.
- Per-leaf output schema version: `aim/v1` on every adopter leaf;
  kit-shipped `spec` and `status` carry kit's own `1.0`.
- Error envelope: `{code, message, cause, suggested_fix, alternatives,
  exit_code}` (kit's `output.Error`); aim-specific codes documented in
  `internal/errs/errs.go`.
- Status sections: `cache`, `source`, `source-breaker`, `identity`,
  `paths`, `environment` (priority 1000+ band) plus kit defaults
  (`profile`, `env`, `workspace`, `auth`, `effective-config`,
  `kit-annotations`).
- `--api-version` negotiation: only `1.0` is honored; the empty value
  defaults to `1.0`. Anything else returns `AIM_INVALID_FLAG` with
  exit code 64.
- Circuit breaker: kit `aim-source` breaker guards
  `ModelsDevSource.Fetch`. State surfaces via `aim status` under the
  `source-breaker` section.
