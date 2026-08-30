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

Environment setup (local toolchains or the devcontainer) and the
full Make target matrix live in [DEVELOPING.md](DEVELOPING.md). The
short version: `make dev-up && make dev-exec` boots a container
with every toolchain preinstalled.

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

This project follows the hop-top
[Code of Conduct](https://github.com/hop-top/.github/blob/main/CODE_OF_CONDUCT.md)
(Contributor Covenant v2.1). Reports: <conduct@hop.top>.
