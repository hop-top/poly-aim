# Contributing to aim

Thanks for your interest in contributing!

## Getting Started

1. Fork the repository
2. Clone your fork locally
3. Create a feature branch: `git checkout -b feat/my-change`
4. Make your changes
5. Run tests: `make test` (Go only) or `make test-all` (all 5 SDKs)
6. Commit using [Conventional Commits](https://conventionalcommits.org)
7. Push and open a Pull Request

## Development Setup

Pick the path that matches what you're touching:

**Local (recommended for single-language work)** — install the toolchain
you need: Go 1.26+, Python 3.11+ (`uv`), Node 22+ (with `--experimental-strip-types`),
Rust stable (`rustup`), PHP 8.2+ with Composer.

**Devcontainer (recommended for multi-language work)** — boots a
preconfigured environment with every toolchain:

```sh
make dev-up           # build + start the container (idempotent)
make dev-exec         # interactive shell inside
make dev-down         # stop + remove (image cache preserved)
make dev-rebuild      # force fresh image build after Dockerfile edits
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

## Code Style

- Follow existing conventions in the codebase
- Run `make lint-all` before submitting
- Keep changes focused; one concern per PR
- Cross-SDK changes must keep `make parity` green

## Commit Messages

Use [Conventional Commits](https://conventionalcommits.org):

```text
feat(scope): add new feature
fix(scope): correct a bug
docs: update readme
test: add missing tests
```

Telegraphese commit bodies preferred (noun phrases, drop articles, min
tokens). One concern per commit.

## Pull Requests

- Reference related issues in the PR description
- Keep PRs small and reviewable
- Ensure `make check-all` passes before requesting review
- Update documentation if behavior changes
- Cross-SDK changes must keep `make parity` green and update
  `docs/sdk-parity.md` if any API moved

## Issues

- Search existing issues before opening a new one
- Use issue templates when available
- Provide reproduction steps for bugs

## Code of Conduct

Be respectful and constructive. We are all here to build something
great together.
