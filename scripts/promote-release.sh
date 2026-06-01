#!/usr/bin/env bash
set -euo pipefail

CONFIG=".github/release-please-config.json"

die() { echo "error: $*" >&2; exit 1; }

command -v jq >/dev/null 2>&1 || die "jq is required but not installed"
[ -f "$CONFIG" ] || die "$CONFIG not found"

usage() {
  cat >&2 <<EOF
usage: $0 [<component>] [<target-stage>]

  <component>     package key (".", "py", "ts", "rs", "php"),
                  component name ("aim", "aim-py", ...), or "all"
                  (default: all)
  <target-stage>  alpha | beta | rc | release (optional; interactive if omitted)

Examples:
  $0                    # interactive, all components
  $0 all                # interactive, all components
  $0 py                 # interactive, py only
  $0 aim-rs             # interactive, rs only (component-name form)
  $0 all beta           # all components -> beta
  $0 rs beta            # rs only -> beta
EOF
  exit 1
}

if ! git diff --cached --quiet; then
  die "staged changes detected. Commit or stash before promoting."
fi

list_components() {
  jq -r '.packages | keys[]' "$CONFIG"
}

component_exists() {
  jq -e --arg c "$1" '.packages | has($c)' "$CONFIG" >/dev/null
}

# resolve_component accepts either the package key (".", "py", ...) or the
# component name ("aim", "aim-py", ...) and prints the canonical package key.
resolve_component() {
  local input="$1"
  if jq -e --arg c "$input" '.packages | has($c)' "$CONFIG" >/dev/null; then
    echo "$input"
    return
  fi
  local matched
  matched=$(jq -r --arg n "$input" '
    .packages | to_entries[]
    | select(.value.component == $n)
    | .key
  ' "$CONFIG")
  if [ -n "$matched" ]; then
    echo "$matched"
    return
  fi
  die "unknown component: $input (known keys: $(list_components | paste -sd ' ' -))"
}

# current_stage prints the prerelease-type stage label (alpha|beta|rc|release)
# for a single package key (e.g. "." or "py"). "release" means no prerelease-type
# is set (stable channel).
current_stage() {
  local c="$1"
  jq -r --arg c "$c" '
    .packages[$c]["prerelease-type"] // "release"
    | sub("\\.[0-9]+$"; "")
  ' "$CONFIG"
}

# all_current_stage returns the shared stage across every package, or "mixed"
# if packages disagree. Used when no component is specified.
all_current_stage() {
  jq -r '
    [ .packages | to_entries[] | (.value["prerelease-type"] // "release") | sub("\\.[0-9]+$"; "") ]
    | unique
    | if length == 1 then .[0] else "mixed" end
  ' "$CONFIG"
}

valid_next() {
  case "$1" in
    release) echo "alpha" ;;
    alpha)   echo "beta" ;;
    beta)    echo "rc" ;;
    rc)      echo "release" ;;
    mixed)   die "components are at mixed stages; promote each individually" ;;
    *)       die "unknown stage: $1" ;;
  esac
}

# stage_value converts a stage label into the prerelease-type value written
# to the config. release-please expects "alpha.0", "beta.0", etc. for the
# initial counter in each channel.
stage_value() {
  case "$1" in
    alpha|beta|rc) echo "$1.0" ;;
    release)       echo "" ;;
    *)             die "unknown stage: $1" ;;
  esac
}

apply_stage_one() {
  local component="$1"
  local stage="$2"
  local tmp
  tmp=$(mktemp)

  if [ "$stage" = "release" ]; then
    jq --arg c "$component" '
      .packages[$c] |= del(.["prerelease-type"])
    ' "$CONFIG" > "$tmp"
  else
    local v
    v=$(stage_value "$stage")
    jq --arg c "$component" --arg s "$v" '
      .packages[$c]["prerelease-type"] = $s
    ' "$CONFIG" > "$tmp"
  fi

  mv "$tmp" "$CONFIG"
}

apply_stage_all() {
  local stage="$1"
  local tmp
  tmp=$(mktemp)

  if [ "$stage" = "release" ]; then
    jq '.packages |= with_entries(
      .value |= del(.["prerelease-type"])
    )' "$CONFIG" > "$tmp"
  else
    local v
    v=$(stage_value "$stage")
    jq --arg s "$v" '.packages |= with_entries(
      .value["prerelease-type"] = $s
    )' "$CONFIG" > "$tmp"
  fi

  mv "$tmp" "$CONFIG"
}

# arg parsing
COMPONENT="${1:-all}"
TARGET="${2:-}"

case "$COMPONENT" in
  -h|--help) usage ;;
esac

if [ "$COMPONENT" != "all" ]; then
  COMPONENT=$(resolve_component "$COMPONENT")
fi

if [ "$COMPONENT" = "all" ]; then
  CURRENT=$(all_current_stage)
else
  CURRENT=$(current_stage "$COMPONENT")
fi

if [ -z "$TARGET" ]; then
  # interactive mode
  NEXT=$(valid_next "$CURRENT")
  echo "Component:     $COMPONENT"
  echo "Current stage: $CURRENT"
  echo "Next stage:    $NEXT"
  printf "Promote to %s? [y/N] " "$NEXT"
  read -r ans
  case "$ans" in
    [yY]*) ;;
    *) echo "Aborted."; exit 0 ;;
  esac
  TARGET="$NEXT"
else
  NEXT=$(valid_next "$CURRENT")
  [ "$TARGET" = "$NEXT" ] || \
    die "invalid transition: $CURRENT -> $TARGET (expected $NEXT)"
fi

if [ "$COMPONENT" = "all" ]; then
  apply_stage_all "$TARGET"
else
  apply_stage_one "$COMPONENT" "$TARGET"
fi

echo "Promoted $COMPONENT to $TARGET"
git add "$CONFIG"
git commit -m "chore(release): promote $COMPONENT to $TARGET" -- "$CONFIG"
