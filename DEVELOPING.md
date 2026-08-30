# Developing

Environment setup and the day-to-day command surface for working on
aim across its five SDKs. Contribution flow (branching, commits,
PRs) lives in [CONTRIBUTING.md](CONTRIBUTING.md); release mechanics
in [RELEASING.md](RELEASING.md).

## Development Setup

Pick the path that matches what you're touching:

**Local (recommended for single-language work)** — install the toolchain
you need: Go 1.26+, Python 3.11+ (`uv`), Node 22+ (with `--experimental-strip-types`),
Rust stable (`rustup`), PHP 8.3+ with Composer (`php/composer.json`
sets the floor).

Host lint tooling (golangci-lint, shellcheck, markdownlint-cli2) is
pinned in `mise.toml` at versions matching CI — `mise install`
provisions all three. The lint targets soft-skip with a hint when a
tool is missing, but findings can drift from CI on unpinned versions.

**Devcontainer (recommended for multi-language work)** — boots a
preconfigured environment with every toolchain:

```sh
make dev-up           # build + start the container (idempotent)
make dev-exec         # interactive shell inside
make dev-down         # stop + remove (image cache preserved)
make dev-rebuild      # force fresh build after devcontainer.json edits
make dev-status       # show container state
make dev-logs         # tail the container logs
```

Run any Make target inside the container:

```sh
make dev-exec CMD="make test-all"
```

Requires Docker. Auto-installs `@devcontainers/cli` via `npx` on first use.

## Make targets

| Target | What runs |
|--------|-----------|
| `make build` | `go build -o bin/aim ./cmd/aim` |
| `make test` / `make test-go` | Go test suite |
| `make test-py` / `make test-ts` / `make test-rs` / `make test-php` | Per-language SDK tests |
| `make test-all` | All 5 SDK test suites |
| `make lint` / `make lint-go` | `go vet ./...` |
| `make lint-py` | Ruff |
| `make lint-ts` | `tsc --noEmit -p tsconfig.json` |
| `make lint-rs` | `cargo fmt --check && cargo clippy -D warnings` |
| `make lint-php` | PHPStan level 8 |
| `make lint-docs` | markdownlint + yaml parse + shellcheck |
| `make lint-all` | All language linters + doc linters |
| `make parity` | Cross-SDK conformance: every SDK passes shared fixtures |
| `make check` | Go-only pre-commit gate: lint + test |
| `make check-all` | Full pre-merge gate: lint-all + test-all + parity |

## Cross-SDK parity

The five SDKs share a wire contract (parser DSL, JSON envelope
shape) pinned by shared fixtures. `make parity` runs every SDK
against them; it must stay green on any cross-SDK change, and
`docs/sdk-parity.md` must be updated if any API moved.
