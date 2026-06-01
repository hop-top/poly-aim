// Package status provides aim-specific [cli.StatusProvider] callbacks
// for the kit-shipped `aim status` subcommand (12-factor State Transparency).
//
// Each exported function returns a [cli.StatusProvider] suitable for
// passing to [cli.Root.RegisterStatusProvider]. Providers are read-only:
// no network calls, no mutation of cache or config.
//
// Section title convention follows kit defaults (lower-case nouns).
// Priority band 1000+ keeps the surface stable against kit-shipped
// providers (which occupy 100-600).
package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"hop.top/aim"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/core/breaker"
	"hop.top/kit/go/core/xdg"
)

// Provider names registered on the kit Root. Exported so callers can
// disable specific providers via [cli.StatusConfig.DisableDefaultProviders]
// or override via [cli.Root.RegisterStatusProvider].
const (
	ProviderCache         = "aim-cache"
	ProviderSource        = "aim-source"
	ProviderSourceBreaker = "aim-source-breaker"
	ProviderIdentity      = "aim-identity"
	ProviderPaths         = "aim-paths"
	ProviderEnvironment   = "aim-environment"
)

// Section titles surfaced in the rendered output. Kept lower-case to
// match kit-shipped section titles (profile, env, workspace, …).
const (
	titleCache         = "cache"
	titleSource        = "source"
	titleSourceBreaker = "source-breaker"
	titleIdentity      = "identity"
	titlePaths         = "paths"
	titleEnvironment   = "environment"
)

// Priority band — keep above kit's 100-600 range.
const (
	priorityCache         = 1000
	prioritySource        = 1010
	prioritySourceBreaker = 1020
	priorityIdentity      = 1100
	priorityPaths         = 1110
	priorityEnvironment   = 1200
)

// envAllowPrefixes lists env-var names (or "PREFIX_*" wildcards) the
// environment provider may surface. Anything outside this list is
// omitted entirely — not redacted — so secrets cannot leak through
// pattern misses.
var envAllowPrefixes = []string{
	"AIM_*",
	"XDG_*",
	"NO_COLOR",
	"GOWORK",
}

// envDenyPatterns blocks any allowlisted key whose name still matches
// a sensitive substring (case-insensitive). Defence-in-depth on top of
// the allowlist.
var envDenyPatterns = []string{
	"TOKEN", "SECRET", "KEY", "PASSWORD",
}

// CacheData is the payload of the cache section. Exported so adopters
// or tests can decode the section without re-deriving field names.
type CacheData struct {
	Dir          string `json:"dir"`
	TTL          string `json:"ttl"`
	LastFetch    string `json:"last_fetch"`
	Age          string `json:"age,omitempty"`
	ETag         string `json:"etag,omitempty"`
	PayloadBytes int64  `json:"payload_bytes,omitempty"`
	PayloadSize  string `json:"payload_size,omitempty"`
	StaleOnError string `json:"stale_on_error"`
}

// SourceData is the payload of the source section.
type SourceData struct {
	URL             string `json:"url"`
	RetrievalMethod string `json:"retrieval_method"`
	Timeout         string `json:"timeout"`
}

// SourceBreakerData is the payload of the source-breaker section. Surfaces
// the live state of the upstream-fetch circuit breaker so operators can
// tell at a glance whether [aim.ModelsDevSource.Fetch] is failing fast.
//
// Fields:
//   - State: "closed" | "open" | "half_open".
//   - Trips: cumulative trip count since process start.
//   - LastStateChange: RFC3339 timestamp of the most recent trip, or
//     "never" when the breaker has never tripped (Closed and clean).
//   - LastTripReason: free-form reason the breaker emitted on its last
//     trip; empty when never tripped.
type SourceBreakerData struct {
	State           string `json:"state"`
	Trips           uint64 `json:"trips"`
	LastStateChange string `json:"last_state_change"`
	LastTripReason  string `json:"last_trip_reason,omitempty"`
}

// IdentityData is the payload of the identity section.
type IdentityData struct {
	AIMVersion string `json:"aim_version"`
	KitVersion string `json:"kit_version"`
	GoVersion  string `json:"go_version,omitempty"`
}

// PathsData is the payload of the paths section.
type PathsData struct {
	ConfigDir string `json:"config_dir"`
	CacheDir  string `json:"cache_dir"`
	DataDir   string `json:"data_dir"`
}

// fileMetaName mirrors aim/cache.go's private filePayload+fileMeta
// constants. Kept aligned with the cache package — if those constants
// move, this mirror must move too.
const (
	cacheFilePayload = "models-dev.json"
	cacheFileMeta    = "meta.json"
)

// metaOnDisk mirrors aim.cacheMeta. We decode it manually because the
// type is unexported by the aim package.
type metaOnDisk struct {
	LastFetch  time.Time `json:"last_fetch"`
	ETag       string    `json:"etag,omitempty"`
	TTLSeconds int64     `json:"ttl_seconds"`
}

// Cache returns the cache-section provider. Resolves the aim cache
// directory via [xdg.CacheDir], reads meta.json + payload size, and
// reports first-run states ("never", missing payload) without panicking.
//
// version is reported only in the source section, not here — kept
// signature param-free so the provider stays trivially registrable.
func Cache() cli.StatusProvider {
	return func(ctx context.Context) (cli.StatusSection, error) {
		sec := cli.StatusSection{
			Title:    titleCache,
			Priority: priorityCache,
			Status:   cli.StatusOK,
		}

		dir, err := resolveCacheDir()
		if err != nil {
			sec.Status = cli.StatusError
			sec.ErrorMessage = err.Error()
			return sec, nil
		}

		data := CacheData{
			Dir:          dir,
			TTL:          (24 * time.Hour).String(),
			LastFetch:    "never",
			StaleOnError: "enabled",
		}

		metaPath := filepath.Join(dir, cacheFileMeta)
		metaBytes, err := os.ReadFile(metaPath)
		switch {
		case err == nil:
			var meta metaOnDisk
			if jerr := json.Unmarshal(metaBytes, &meta); jerr == nil {
				if !meta.LastFetch.IsZero() {
					data.LastFetch = meta.LastFetch.UTC().Format(time.RFC3339)
					data.Age = time.Since(meta.LastFetch).Round(time.Second).String()
				}
				if meta.TTLSeconds > 0 {
					data.TTL = (time.Duration(meta.TTLSeconds) * time.Second).String()
				}
				if meta.ETag != "" {
					data.ETag = truncate(meta.ETag, 12)
				}
			}
		case errors.Is(err, os.ErrNotExist):
			// First run — keep defaults.
		default:
			// Unexpected I/O error — surface but don't fail boot.
			sec.Status = cli.StatusError
			sec.ErrorMessage = fmt.Sprintf("read meta.json: %v", err)
		}

		payloadPath := filepath.Join(dir, cacheFilePayload)
		if info, perr := os.Stat(payloadPath); perr == nil {
			data.PayloadBytes = info.Size()
			data.PayloadSize = humanBytes(info.Size())
		} else {
			data.PayloadSize = "unknown"
		}

		sec.Data = data
		return sec, nil
	}
}

// Source returns the source-section provider. Reports the default
// upstream URL, the retrieval verb, and the documented timeout. Reads
// nothing from the network or disk.
func Source() cli.StatusProvider {
	return func(ctx context.Context) (cli.StatusSection, error) {
		return cli.StatusSection{
			Title:    titleSource,
			Priority: prioritySource,
			Status:   cli.StatusOK,
			Data: SourceData{
				URL:             aim.DefaultSourceURL,
				RetrievalMethod: "http_get",
				Timeout:         (30 * time.Second).String(),
			},
		}, nil
	}
}

// SourceBreaker returns the source-breaker section provider. Reads the
// process-wide breaker registered under [aim.SourceBreakerName]; when
// absent (e.g. the upstream fetcher was never instantiated) the section
// reports State="closed", Trips=0, LastStateChange="never" so the wire
// shape stays stable across cold-start invocations.
func SourceBreaker() cli.StatusProvider {
	return func(ctx context.Context) (cli.StatusSection, error) {
		sec := cli.StatusSection{
			Title:    titleSourceBreaker,
			Priority: prioritySourceBreaker,
			Status:   cli.StatusOK,
		}
		data := SourceBreakerData{
			State:           "closed",
			LastStateChange: "never",
		}
		if b, ok := breaker.Lookup(aim.SourceBreakerName); ok {
			data.State = b.State().String()
			s := b.Stats()
			data.Trips = s.Trips
			if !s.LastTripAt.IsZero() {
				data.LastStateChange = s.LastTripAt.UTC().Format(time.RFC3339)
			}
			data.LastTripReason = s.LastTripReason
		}
		sec.Data = data
		return sec, nil
	}
}

// Identity returns the identity-section provider. aimVersion is the
// caller-supplied semver string (typically [cli.Config.Version]); kit
// version is resolved at runtime from build info.
func Identity(aimVersion string) cli.StatusProvider {
	return func(ctx context.Context) (cli.StatusSection, error) {
		if aimVersion == "" {
			aimVersion = "dev"
		}
		data := IdentityData{
			AIMVersion: aimVersion,
			KitVersion: lookupKitVersion(),
		}
		if bi, ok := debug.ReadBuildInfo(); ok {
			data.GoVersion = bi.GoVersion
		}
		return cli.StatusSection{
			Title:    titleIdentity,
			Priority: priorityIdentity,
			Status:   cli.StatusOK,
			Data:     data,
		}, nil
	}
}

// Paths returns the paths-section provider. Surfaces the XDG-resolved
// config/cache/data directories. Uses Raw* variants — pattern lookup
// only, no I/O — so the section never trips the package guard.
func Paths() cli.StatusProvider {
	return func(ctx context.Context) (cli.StatusSection, error) {
		data := PathsData{}
		if v, err := xdg.RawConfigDir("aim"); err == nil {
			data.ConfigDir = v
		}
		if v, err := xdg.RawCacheDir("hop"); err == nil {
			data.CacheDir = filepath.Join(v, "aim")
		}
		if v, err := xdg.RawDataDir("aim"); err == nil {
			data.DataDir = v
		}
		return cli.StatusSection{
			Title:    titlePaths,
			Priority: priorityPaths,
			Status:   cli.StatusOK,
			Data:     data,
		}, nil
	}
}

// Environment returns the environment-section provider. Only env vars
// matching [envAllowPrefixes] surface; anything else is dropped. Within
// the allowlist, names hitting [envDenyPatterns] are redacted to
// "[redacted]". Empty values render as "(unset)" so the section length
// stays stable across machines.
func Environment() cli.StatusProvider {
	return func(ctx context.Context) (cli.StatusSection, error) {
		sec := cli.StatusSection{
			Title:     titleEnvironment,
			Priority:  priorityEnvironment,
			Sensitive: false,
			Status:    cli.StatusOK,
		}
		out := map[string]string{}
		for _, prefix := range envAllowPrefixes {
			collectEnv(prefix, out)
		}
		if len(out) == 0 {
			sec.Status = cli.StatusEmpty
			return sec, nil
		}
		sec.Data = out
		return sec, nil
	}
}

// Register attaches every aim status provider to root. Convenience
// wrapper for cmd/aim/main.go — the only place that should know the
// full provider list.
func Register(root *cli.Root, aimVersion string) {
	if root == nil {
		return
	}
	root.RegisterStatusProvider(ProviderCache, Cache())
	root.RegisterStatusProvider(ProviderSource, Source())
	root.RegisterStatusProvider(ProviderSourceBreaker, SourceBreaker())
	root.RegisterStatusProvider(ProviderIdentity, Identity(aimVersion))
	root.RegisterStatusProvider(ProviderPaths, Paths())
	root.RegisterStatusProvider(ProviderEnvironment, Environment())
}

// resolveCacheDir mirrors aim.Cache.dir() — the default path the cache
// uses when WithCacheDir was not passed. Kept in sync with aim/cache.go.
func resolveCacheDir() (string, error) {
	base, err := xdg.RawCacheDir("hop")
	if err != nil {
		return "", fmt.Errorf("aim status: cache dir: %w", err)
	}
	return filepath.Join(base, "aim"), nil
}

// lookupKitVersion walks debug.BuildInfo.Deps for the hop.top/kit
// module version. Returns "unknown" when build info is missing
// (e.g. binary built without modules) or kit is not a recorded dep.
func lookupKitVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range bi.Deps {
		if dep == nil {
			continue
		}
		if dep.Path == "hop.top/kit" {
			if dep.Replace != nil && dep.Replace.Version != "" {
				return dep.Replace.Version
			}
			return dep.Version
		}
	}
	// Fall back to the main module when kit shows up as the main
	// (e.g. kit's own test binary).
	if bi.Main.Path == "hop.top/kit" {
		return bi.Main.Version
	}
	return "unknown"
}

// collectEnv harvests os.Environ() entries matching the pattern.
// Pattern is "EXACT_NAME" or "PREFIX_*" (trailing star). Matches go
// through [envDenyPatterns] before landing in out.
func collectEnv(pattern string, out map[string]string) {
	for _, e := range os.Environ() {
		eq := strings.IndexByte(e, '=')
		if eq < 0 {
			continue
		}
		name := e[:eq]
		value := e[eq+1:]
		if !matchEnvPattern(name, pattern) {
			continue
		}
		if matchAnyDeny(name) {
			out[name] = "[redacted]"
			continue
		}
		if value == "" {
			out[name] = "(unset)"
			continue
		}
		out[name] = value
	}
}

func matchEnvPattern(name, pattern string) bool {
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
	}
	return name == pattern
}

func matchAnyDeny(name string) bool {
	upper := strings.ToUpper(name)
	for _, p := range envDenyPatterns {
		if strings.Contains(upper, p) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	suffixes := []string{"KiB", "MiB", "GiB", "TiB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), suffixes[exp])
}
