# Releasing

## Components

aim is a multi-component repo. Each language SDK is released
independently:

| Path  | Component | Tag prefix     | Channel    |
|-------|-----------|----------------|------------|
| `.`   | `aim`     | `aim/v...`     | go module  |
| `py/` | `aim-py`  | `aim-py/v...`  | PyPI       |
| `ts/` | `aim-ts`  | `aim-ts/v...`  | npm        |
| `rs/` | `aim-rs`  | `aim-rs/v...`  | crates.io  |
| `php/`| `aim-php` | `aim-php/v...` | Packagist  |

Commit scopes are detected by file path. A commit touching
`py/src/hop/aim/types.py` opens a release-pending PR for `aim-py` only;
`rs/src/types.rs` opens one for `aim-rs`. The Go module (`.`) is bumped
by commits anywhere outside `py/`, `ts/`, `rs/`, `php/`.

For coordinated releases across all five components, manually open
release PRs for each component via release-please's `release-pr` action
with `--component <name>`.

## Version Lifecycle

```text
0.1.0-alpha.0 -> .1 -> ... -> 0.1.0-beta.0 -> ... -> 0.1.0-rc.0 -> ... -> 0.1.0
```

| Stage   | Audience     | API              | Breaking changes  |
|---------|--------------|------------------|-------------------|
| alpha   | contributors | unstable         | expected          |
| beta    | testers      | feature-complete | only if critical  |
| rc      | everyone     | frozen           | showstoppers only |
| release | everyone     | stable           | next major only   |

All five components seed at `0.1.0-alpha.0`. Each one advances through
the lifecycle independently via release-please prerelease bumps on the
`alpha.N` / `beta.N` / `rc.N` counters.

## How releases work

1. Conventional commits land on `main`
2. release-please opens / updates a release PR per affected component
   (`separate-pull-requests: true` — one standing PR per component,
   never a combined one)
3. Merging the release PR cuts the GitHub Release + `<component>/v<version>` tag
4. `.github/workflows/publish.yml` fires on the tag push and delegates
   to the org-shared `hop-top/.github` reusable workflow

The release workflow lives at `.github/workflows/release-please.yml`.
It mints a short-lived installation token from the org release-bot
GitHub App (`RELEASE_BOT_APP_ID` + `RELEASE_BOT_PRIVATE_KEY`) so
release PRs come from `release-bot[bot]` — long-lived PATs are
rejected by the preflight. It supports `workflow_dispatch` for manual
retrigger after sibling-PR conflicts.

### Release PR mechanics

- **Approval brake**: release PRs rewrite
  `.release-please-manifest.json` + `CHANGELOG.md`, which are owned by
  `@hop-top/release` in CODEOWNERS. The `production-branch-guardrail`
  ruleset (0 required approvals + code-owner review) means release PRs
  need an explicit release-team approval; everything else auto-merges
  freely.
- **Merge style**: squash single-commit release PRs (a plain merge
  leaves an unparseable `Merge pull request #N` subject and
  release-please never tags).
- **Companion tagger**: `.github/workflows/release-tag.yml` tags
  `<component>/v<version>` at the merge commit and flips the status
  label. It is idempotent — with native GitHub Releases on,
  release-please tags first and the companion harmlessly loses the
  race.
- **Native Releases stay on**: `skip-github-release` is deliberately
  NOT set. release-please anchors commit ranges on Release objects,
  so tag-only releases make standing PRs re-list already-shipped
  commits (googleapis/release-please#1295). Until upstream supports
  tag-based lookup, Releases remain on this repo and the companion
  sits ready.

### Preflight gate

`release-please-preflight.yml` runs on every PR that touches the
release-please / publish surface (config, manifest, the two
workflow files, or any of the per-language version manifests). It
delegates to
`hop-top/.github/.github/workflows/release-please-preflight.yml@v0`
which runs 30+ static checks against the config + manifest +
workflow shapes and surfaces resolution hints when something is
misconfigured. Trigger manually via the Actions tab if needed.

Local equivalents:

- `make release-validate` — offline config/manifest agreement check
- `make release-dry` — dry-run release-please against the live commit
  history (requires `GITHUB_TOKEN` or `gh` auth)

### What happens after the tag

Aim's single `publish.yml` calls
`hop-top/.github/.github/workflows/publish-on-tag.yml@v0` with an
`ecosystems:` map. The reusable workflow parses the tag's
`<component>/v<version>` prefix, looks up the component in the map,
and routes to the appropriate per-ecosystem reusable workflow.

| Tag pattern      | Routed to          | Action                                              |
|------------------|--------------------|-----------------------------------------------------|
| `aim/v*`         | (mirror only)      | Subtree-push to `hop-top/aim`; proxy.golang.org + the hop.top vanity resolver serve from the mirror |
| `aim-py/v*`      | `publish-py.yml`   | Build + publish to PyPI via token auth              |
| `aim-ts/v*`      | `publish-ts.yml`   | `pnpm publish` to npm (aim's ts uses `--no-frozen-lockfile`) |
| `aim-rs/v*`      | `publish-rs.yml`   | `cargo publish` to crates.io                        |
| `aim-php/v*`     | (mirror + notify)  | Subtree-push to mirror, then notify Packagist       |

Every component gets a subtree push to a read-only mirror repo so
consumers can target the language-specific repo directly:
`hop-top/aim-py`, `hop-top/aim-ts`, etc.; Go takes the bare name
(`hop-top/aim` — the module URL is the repo path). The hop.top
vanity worker always resolves `go get hop.top/aim` to the mirror —
no repo-side notify step (the old `notify-vanity` mechanism is
retired).

Mirror push is gated on publish success: if `publish-rs` fails,
`mirror` does not run, and the Packagist notify (PHP) is
short-circuited.

`publish.yml` also supports `workflow_dispatch` to re-run a publish
for an existing tag without re-pushing it. Caveat: dispatch replays
the workflow file at the tag's commit, not main HEAD.

### Required GitHub secrets

All secrets are org-level on `hop-top`. They MUST be available to
this repo (org → Settings → Secrets → Repository access). If a
secret is missing the publish workflow fails loudly and the tag is
preserved — the publish can be retried via "Re-run workflow" once
the secret is in place.

| Secret                     | Required for                | Provisioned at |
|----------------------------|-----------------------------|----------------|
| `RELEASE_BOT_APP_ID`       | `release-please.yml`, `release-tag.yml` (NOT `publish.yml` — the reusable no longer declares it) | hop-top org App registration |
| `RELEASE_BOT_PRIVATE_KEY`  | same as `RELEASE_BOT_APP_ID` | hop-top org App private key |
| `GH_MIRROR_PAT`            | Mirror subtree push (all SDK components) | GitHub PAT with `repo` + `workflow` on mirror repos |
| `PYPI_REGISTRY_TOKEN`      | `aim-py/v*`                 | [pypi.org](https://pypi.org/manage/account/token/) |
| `NPM_REGISTRY_TOKEN`       | `aim-ts/v*`                 | npmjs.com automation token |
| `CARGO_REGISTRY_TOKEN`     | `aim-rs/v*`                 | [crates.io](https://crates.io/settings/tokens) |
| `PACKAGIST_USERNAME`       | `aim-php/v*` notify         | packagist.org account |
| `PACKAGIST_TOKEN`          | `aim-php/v*` notify         | [packagist.org](https://packagist.org/profile/) API token |

The publish workflow is a thin delegating shell over the org-shared
reusable workflow. Behavior changes (new ecosystem, default-flag
tweaks, etc.) land in `hop-top/.github`, not here. Local overrides
go in the `ecosystems:` map per component.

## Promoting a release stage

Aim ships a helper script for stage transitions:

```bash
./scripts/promote-release.sh                 # interactive, all components
./scripts/promote-release.sh all             # interactive, all components
./scripts/promote-release.sh py              # interactive, py only
./scripts/promote-release.sh aim-rs          # rs only (component name)
./scripts/promote-release.sh all beta        # all components -> beta
./scripts/promote-release.sh rs beta         # rs only -> beta
```

The script edits `.github/release-please-config.json` to change the
`prerelease-type` (e.g. `alpha.0` -> `beta.0`) and commits the change.
release-please picks up the new channel on the next PR.

Stages: `release -> alpha -> beta -> rc -> release`. Each step is
one-way; promoting `rs` from `alpha` to `rc` directly is rejected.

### Transition criteria

| Transition       | Criteria                         |
|------------------|----------------------------------|
| release -> alpha | new version cycle starts         |
| alpha -> beta    | all planned features merged      |
| beta -> rc       | no known bugs blocking release   |
| rc -> release    | bake period passed, no regressions |

## Preconditions

- Zero existing tags before the first release-please PR merge.
  `include-component-in-tag` is `true` and the toggle is one-way;
  pre-existing tags would break.
- `make check-all` passes locally + in CI on the commit being released.
- For coordinated multi-SDK releases: `make parity` green.

## Cross-SDK changes

When a release touches multiple SDKs (e.g. adding a field to the
shared wire format), promote all five components together — they
travel through stages in lockstep until the API stabilizes.

After the first stable release, components MAY drift in version, but
the cross-SDK contract (parser DSL, JSON envelope shape) MUST stay
aligned. The `make parity` gate enforces this.
