#!/usr/bin/env bash
# Runs every SDK's cross-SDK test suite. Used by humans + a future CI gate.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "==> Go"
GOWORK=off go test -buildvcs=false -count=1 -short ./... > /dev/null
echo "    ok"

echo "==> Python"
# pyproject.toml exposes pytest via the [test] optional-dependency group;
# bare `uv run pytest` cannot find pytest without --extra test.
( cd py && uv run --extra test pytest tests/test_query_vectors.py -q )
echo "    ok"

echo "==> TypeScript"
# Node >=22 supports --experimental-strip-types for .ts test files.
# If on older Node, swap to: npx -y tsx --test src/query_vectors.test.ts
( cd ts && node --experimental-strip-types --test src/query_vectors.test.ts )
echo "    ok"

echo "==> Rust"
# First run compiles the crate + dev-deps (~30s cold); subsequent runs cached.
( cd rs && cargo test --test query_vectors --test registry_vectors --quiet )
echo "    ok"

echo "==> PHP"
# Bootstrap vendor/ on first run; composer install is idempotent so skip when
# vendor/ already exists to avoid the network round-trip.
[ -d php/vendor ] || ( cd php && composer install --no-interaction --no-progress >/dev/null )
( cd php && vendor/bin/phpunit --testdox tests/QueryVectorsTest.php tests/RegistryVectorsTest.php )
echo "    ok"

echo
echo "All 5 SDKs in parity."
