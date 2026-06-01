# aim — top-level Makefile
#
# Per-language: test-{go,py,ts,rs,php}, lint-{go,py,ts,rs,php}.
# Aggregated: test-all, lint-all, check-all, parity.
# Bare `make test` / `make lint` stay Go-only (canonical implementation).
#
# go.work pulls in kit from a different git root; suppress VCS stamp errors.
export GOFLAGS := -buildvcs=false

.PHONY: build test test-verbose lint check smoke install \
        test-go test-py test-ts test-rs test-php test-all \
        lint-go lint-py lint-ts lint-rs lint-php lint-all \
        lint-md lint-yaml lint-sh lint-docs \
        parity conformance install-py install-php install-ts check-all \
        dev-up dev-down dev-exec dev-rebuild dev-status dev-logs \
        release-dry release-validate promote

# --- Go (canonical) ---

build:
	go build -o bin/aim ./cmd/aim

test test-go:
	go test ./...

test-verbose:
	go test -v ./...

lint lint-go:
	go vet ./...
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || \
		echo "lint-go: golangci-lint not installed (CI uses v2.12.2); 'mise install' to enable"

smoke:
	go test -tags smoke ./...

install:
	go install ./cmd/aim

# --- Python (uv + pytest + ruff) ---

test-py:
	cd py && uv run --extra test pytest

# ruff is not in py/pyproject.toml's [test] extra; pull it on demand via --with.
lint-py:
	cd py && uv run --with ruff ruff check .

install-py:
	cd py && uv sync --extra test

# --- TypeScript (node native test runner) ---

test-ts:
	cd ts && node --experimental-strip-types --test src/query_vectors.test.ts

# Lint via tsc strict mode. Local devDep first; falls back to global tsc.
lint-ts: install-ts
	cd ts && npx --no-install tsc --noEmit -p tsconfig.json

install-ts:
	@[ -d ts/node_modules ] || ( cd ts && npm install --no-audit --no-fund )

# --- Rust (cargo) ---

test-rs:
	cd rs && cargo test

lint-rs:
	cd rs && cargo fmt --check && cargo clippy --all-targets -- -D warnings

# --- PHP (phpunit + phpstan) ---

install-php:
	cd php && composer install --no-interaction --no-progress

test-php:
	@[ -d php/vendor ] || $(MAKE) install-php
	cd php && vendor/bin/phpunit

lint-php:
	@[ -d php/vendor ] || $(MAKE) install-php
	cd php && vendor/bin/phpstan analyse src --level=8 --no-progress

# --- Docs / config files ---

# Markdown: markdownlint-cli2. Config in .markdownlint-cli2.jsonc at repo root.
lint-md:
	@if command -v markdownlint-cli2 >/dev/null 2>&1; then \
		markdownlint-cli2 "**/*.md"; \
	else \
		echo "lint-md: markdownlint-cli2 not installed; 'brew install markdownlint-cli2' to enable"; \
	fi

# YAML: parse-level check via python3 (yamllint is optional; not on every box).
lint-yaml:
	@python3 -c "import sys, yaml, pathlib; \
		paths = [p for p in pathlib.Path('.').rglob('*.y*ml') if not any(x in p.parts for x in ('node_modules','vendor','target','dist','.kit'))]; \
		[yaml.safe_load(p.read_text()) for p in paths]; \
		print(f'lint-yaml: {len(paths)} files parsed clean')"

# Shell: shellcheck on all tracked scripts.
lint-sh:
	@if command -v shellcheck >/dev/null 2>&1; then \
		shellcheck scripts/*.sh .devcontainer/post-create.sh; \
	else \
		echo "lint-sh: shellcheck not installed; 'brew install shellcheck' to enable"; \
	fi

lint-docs: lint-md lint-yaml lint-sh

# --- Release / release-please ---
#
# release-please runs in CI on push to main. These targets are for
# previewing what it WOULD do locally, before the actual workflow runs.
# Requires npx (auto-installs release-please-action on first use).

RELEASE_PLEASE := npx -y release-please@latest
RELEASE_CONFIG := .github/release-please-config.json
RELEASE_MANIFEST := .github/.release-please-manifest.json
RELEASE_REPO := hop-top/aim

# release-dry shows what release PRs release-please WOULD open against
# main right now. Requires GITHUB_TOKEN with repo:read scope — without
# it the GitHub API call fails before release-please can compute the
# diff. Get a token with `gh auth token` if gh is installed.
release-dry:
	@command -v npx >/dev/null 2>&1 || { echo "release-dry: npx (node) required"; exit 1; }
	@if [ -z "$$GITHUB_TOKEN" ]; then \
		if command -v gh >/dev/null 2>&1; then \
			tok=$$(gh auth token 2>/dev/null); \
			if [ -n "$$tok" ]; then \
				GITHUB_TOKEN="$$tok" $(MAKE) release-dry; exit $$?; \
			fi; \
		fi; \
		echo "release-dry: set GITHUB_TOKEN or run 'gh auth login' first"; \
		exit 1; \
	fi
	$(RELEASE_PLEASE) release-pr \
		--dry-run \
		--token "$$GITHUB_TOKEN" \
		--repo-url $(RELEASE_REPO) \
		--config-file $(RELEASE_CONFIG) \
		--manifest-file $(RELEASE_MANIFEST)

# release-validate checks the config + manifest parse and agree on
# component keys. Fast, no network, no token required.
release-validate:
	@python3 -c "import json, sys; \
		cfg = json.load(open('$(RELEASE_CONFIG)')); \
		mf  = json.load(open('$(RELEASE_MANIFEST)')); \
		ck = set(cfg['packages'].keys()); \
		mk = set(mf.keys()); \
		assert ck == mk, f'package key mismatch: config={ck} manifest={mk}'; \
		print(f'release-validate: {len(ck)} components, config + manifest agree')"

# promote wraps scripts/promote-release.sh. Pass COMPONENT and STAGE
# as Make variables: make promote COMPONENT=aim-rs STAGE=beta
# Interactive mode if either is omitted.
promote:
	./scripts/promote-release.sh $(COMPONENT) $(STAGE)

# --- Aggregates ---

test-all: test-go test-py test-ts test-rs test-php

lint-all: lint-go lint-py lint-ts lint-rs lint-php lint-docs

# --- Devcontainer lifecycle ---
#
# Uses @devcontainers/cli (Microsoft's reference implementation).
# Auto-installed on first use via npx; no global install required.
# Requires Docker.

DEVCONTAINER := npx -y @devcontainers/cli@latest
DEVCONTAINER_CONFIG := .devcontainer/devcontainer.json

# dev-up builds (if needed) and starts the container. Idempotent —
# subsequent invocations re-use the running container.
dev-up:
	@command -v docker >/dev/null 2>&1 || { echo "dev-up: docker is required"; exit 1; }
	$(DEVCONTAINER) up --workspace-folder . --config $(DEVCONTAINER_CONFIG)

# dev-exec opens an interactive shell inside the running container.
# Use `make dev-exec CMD="go test ./..."` to run a specific command.
dev-exec:
	@command -v docker >/dev/null 2>&1 || { echo "dev-exec: docker is required"; exit 1; }
	$(DEVCONTAINER) exec --workspace-folder . --config $(DEVCONTAINER_CONFIG) $(if $(CMD),$(CMD),bash)

# dev-down stops + removes the container. Image cache is preserved;
# next dev-up reuses it for fast restart.
dev-down:
	@command -v docker >/dev/null 2>&1 || { echo "dev-down: docker is required"; exit 1; }
	@cid=$$(docker ps -aq --filter "label=devcontainer.local_folder=$$PWD"); \
	if [ -n "$$cid" ]; then docker rm -f $$cid; else echo "dev-down: no container for $$PWD"; fi

# dev-rebuild forces a fresh image build. Use after editing Dockerfile.
dev-rebuild:
	@command -v docker >/dev/null 2>&1 || { echo "dev-rebuild: docker is required"; exit 1; }
	$(MAKE) dev-down
	$(DEVCONTAINER) up --workspace-folder . --config $(DEVCONTAINER_CONFIG) --remove-existing-container

dev-status:
	@docker ps -a --filter "label=devcontainer.local_folder=$$PWD" \
		--format "table {{.Names}}\t{{.Status}}\t{{.Image}}" \
		2>/dev/null || echo "dev-status: docker not available"

dev-logs:
	@cid=$$(docker ps -aq --filter "label=devcontainer.local_folder=$$PWD"); \
	if [ -n "$$cid" ]; then docker logs --tail 100 $$cid; else echo "dev-logs: no container for $$PWD"; fi

# parity runs the cross-SDK verification: every SDK passes the same
# query-vectors + registry-vectors fixtures.
parity:
	./scripts/verify-sdk-parity.sh

# conformance runs every 12-factor test in isolation, then regenerates
# docs/12-factor-conformance.md. Commit both the test results and the
# regenerated report.
conformance:
	go test -count=1 -run TestFactor ./internal/conformance/...
	go test -count=1 -run TestGenerateReport ./internal/conformance/...

# check stays Go-only (cheap pre-commit gate).
# check-all gates on every language + parity (slower, pre-merge).
check: lint test

check-all: lint-all test-all parity
