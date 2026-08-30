#!/usr/bin/env bash
# post-create.sh — runs after devcontainer create. Mirrors the Makefile's
# per-SDK install steps so `make test-all` / `make lint-all` work out of
# the box inside the container.
set -euo pipefail

echo "==> Installing uv (Python runner used by make test-py)"
if ! command -v uv >/dev/null 2>&1; then
  curl -LsSf https://astral.sh/uv/install.sh | sh
fi
export PATH="$HOME/.local/bin:$PATH"

echo "==> Warming Go module cache"
go mod download

echo "==> Installing Python dependencies"
(cd py && uv sync --extra test)

echo "==> Installing TypeScript dependencies"
(cd ts && npm install --no-audit --no-fund)

echo "==> Installing PHP dependencies"
(cd php && composer install --no-interaction --no-progress)

echo "==> Warming Rust toolchain"
(cd rs && cargo fetch)

echo "Dev environment ready. Run: make check-all"
