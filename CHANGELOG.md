# Changelog

## [0.1.0-alpha.3](https://github.com/hop-top/poly-aim/compare/aim/v0.1.0-alpha.2...aim/v0.1.0-alpha.3) (2026-08-30)


### ⚠ BREAKING CHANGES

* exit codes remapped — NOT_FOUND 1→3, INVALID_QUERY 64→2, AIM_INVALID_FLAG 64→2, AIM_NETWORK 2→6, AIM_SOURCE_UNAVAILABLE 2→6, AIM_CACHE_CORRUPT 2→1, AIM_CACHE_LOCKED 2→4. Process exit code now matches the envelope's exit_code (previously always 1 on error).

### Bug Fixes

* remap exit codes to shared taxonomy ([#29](https://github.com/hop-top/poly-aim/issues/29)) ([0d9811f](https://github.com/hop-top/poly-aim/commit/0d9811f95b2b78478f83f005eb8f8574f96279a5))

## [0.1.0-alpha.2](https://github.com/hop-top/poly-aim/compare/aim/v0.1.0-alpha.1...aim/v0.1.0-alpha.2) (2026-06-07)


### Bug Fixes

* **py,rs:** trigger release for hop-top-aim rename ([#7](https://github.com/hop-top/poly-aim/issues/7)) ([8782452](https://github.com/hop-top/poly-aim/commit/8782452d16dc96adbe17aa71ff0f61af869c2ed2))

## [0.1.0-alpha.1](https://github.com/hop-top/poly-aim/compare/aim/v0.1.0-alpha.0...aim/v0.1.0-alpha.1) (2026-06-06)


### Bug Fixes

* **py:** add pytest pythonpath so src package resolves ([#2](https://github.com/hop-top/poly-aim/issues/2)) ([342c176](https://github.com/hop-top/poly-aim/commit/342c17691e7a4a6d5881bec55e4cb44ef8d3e682))

## [0.1.0-alpha.0](https://github.com/hop-top/poly-aim/compare/aim/v0.1.0-alpha.0...aim/v0.1.0-alpha.0) (2026-06-02)


### Miscellaneous

* initial public release ([c63a57c](https://github.com/hop-top/poly-aim/commit/c63a57c2056199a24045ead9e6e351e847710736))
