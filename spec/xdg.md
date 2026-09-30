# XDG Cache Directory Resolution

Algorithm used by all `aim` ports for locating the cache directory.

## Precedence

1. `XDG_CACHE_HOME` env var (if set and non-empty) → `$XDG_CACHE_HOME/hop/aim/`
2. Platform default:
   - **macOS** → `~/Library/Caches/hop/aim/`
   - **Windows** → `%LOCALAPPDATA%\hop\aim\`
   - **Linux** → `~/.cache/hop/aim/`
3. Fallback → `~/.cache/hop/aim/`

## Files written

```text
<cache-dir>/
  models-dev.json   — cached api.json payload
  meta.json         — {"last_fetch": <RFC3339>, "etag": "<string>", "ttl_seconds": 86400}
  .lock             — sentinel lockfile (covers full fetch+write cycle)
```

## Port implementation notes

| Language | Cache dir function |
|----------|--------------------|
| Go       | `hop.top/kit/xdg.CacheDir("hop")` → append `/aim` |
| TS       | `os.homedir()` + platform check or `$XDG_CACHE_HOME` |
| Python   | `platformdirs.user_cache_dir("hop", appauthor=False)` + `/aim` |
