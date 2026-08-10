// Package errs builds aim's structured error envelopes.
//
// Constructors return kit's *output.Error so the cli.WrapRunE middleware
// can render the envelope to stderr in the active --format. The catalog
// (Factor 4), with the exit code each code maps to:
//
//   - NOT_FOUND           (3) — missing provider/model in the local cache.
//   - INVALID_QUERY       (2) — aim.ParseQuery rejects the query expression.
//   - AIM_INVALID_FLAG    (2) — flag value out of range past cobra's check.
//   - AIM_NETWORK         (6) — HTTP transport failure during refresh.
//   - AIM_CACHE_CORRUPT   (1) — meta.json or payload JSON unreadable.
//   - AIM_SOURCE_UNAVAILABLE (6) — upstream 5xx / context-deadline-exceeded.
//   - AIM_CACHE_LOCKED    (4) — concurrent refresh holds the lockfile.
//
// Exit codes follow the shared taxonomy: 0 success, 1 general, 2 usage,
// 3 not-found, 4 conflict, 5 permission, 6 transient/retryable,
// 64 rate-limited. Where kit ships a constructor for a class
// (output.NotFoundError, output.UsageError, output.ConflictError), the
// envelope is built on top of it so the exit code tracks kit's table.
// AIM-specific codes are declared below with the AIM_ prefix so
// cross-tool log aggregation can disambiguate.
package errs

import (
	"fmt"
	"strings"

	"hop.top/kit/go/console/output"
)

// AIM-specific error codes. Kit-shipped codes (CodeNotFound, CodeUsage,
// etc.) are reused via output.* — these constants cover gaps in the kit
// catalog so the aim envelope stays stable across releases.
const (
	// CodeInvalidQuery — aim.ParseQuery rejects the query expression.
	// Maps to exit 2 (usage).
	CodeInvalidQuery = "INVALID_QUERY"
	// CodeInvalidFlag — flag value out of range past cobra's check.
	// Maps to exit 2 (usage).
	CodeInvalidFlag = "AIM_INVALID_FLAG"
	// CodeNetwork — HTTP transport failure during refresh.
	// Maps to exit 6 (transient).
	CodeNetwork = "AIM_NETWORK"
	// CodeCacheCorrupt — meta.json or payload JSON unreadable.
	// Maps to exit 1 (general).
	CodeCacheCorrupt = "AIM_CACHE_CORRUPT"
	// CodeSourceUnavailable — upstream 5xx or context-deadline-exceeded.
	// Maps to exit 6 (transient).
	CodeSourceUnavailable = "AIM_SOURCE_UNAVAILABLE"
	// CodeCacheLocked — concurrent refresh holds the lockfile.
	// Maps to exit 4 (conflict).
	CodeCacheLocked = "AIM_CACHE_LOCKED"
)

// Exit codes for classes kit ships no constructor for. Classes kit
// covers (usage=2, not-found=3, conflict=4) come from the kit
// constructors used below and are not repeated here.
const (
	// exitGeneric classifies unrecoverable local failures (corrupt
	// cache) where neither retry nor a caller-side fix applies.
	exitGeneric = 1
	// exitTransient classifies retryable failures — transport errors
	// and upstream unavailability where backing off and retrying the
	// same invocation may succeed.
	exitTransient = 6
)

// NotFound returns an *output.Error for a missing provider/model lookup.
// kind is the noun ("provider", "model"); value is what was searched.
func NotFound(kind, value string) *output.Error {
	msg := fmt.Sprintf("%s not found: %s", kind, value)
	alts := []string{
		"aim providers",
		"aim list",
		"aim refresh",
	}
	if kind == "model" {
		alts = []string{
			"aim list --provider <provider>",
			"aim providers",
			"aim refresh",
		}
	}
	e := output.NotFoundError(msg)
	e.SuggestedFix = suggestedFixNotFound(kind)
	e.Alternatives = alts
	return e
}

func suggestedFixNotFound(kind string) string {
	switch kind {
	case "provider":
		return "run `aim providers` to list valid provider IDs, " +
			"or `aim refresh` if the local cache is stale"
	case "model":
		return "run `aim list --provider <provider>` to list valid " +
			"model IDs, or `aim refresh` if the local cache is stale"
	default:
		return "run `aim providers` to list valid providers and " +
			"`aim list` to list models"
	}
}

// InvalidQuery wraps a ParseQuery error in the envelope. expr is the
// rejected query string; cause is the underlying parser error.
func InvalidQuery(expr string, cause error) *output.Error {
	msg := fmt.Sprintf("invalid query: %s", expr)
	causeMsg := ""
	if cause != nil {
		causeMsg = cause.Error()
	}
	e := output.UsageError(msg)
	e.Code = CodeInvalidQuery
	e.Cause = causeMsg
	e.SuggestedFix = "use quote-aware tag syntax (provider:<id>, " +
		"in:<modality>, tool_call:true|false). " +
		"See `aim spec --format json | jq '.commands[] | " +
		"select(.path[1] == \"query\") | .examples'`"
	e.Alternatives = []string{
		`aim query "provider:openai tool_call:true"`,
		`aim query "in:image,text reasoning:true"`,
		"aim list --help",
	}
	return e
}

// InvalidFlag wraps a flag-validation failure outside cobra's reach.
// flag is the bare flag name (no dashes); value is the rejected value;
// detail is a human-readable reason ("must be one of: …").
func InvalidFlag(flag, value, detail string) *output.Error {
	msg := fmt.Sprintf("invalid value for --%s: %q", flag, value)
	e := output.UsageError(msg)
	e.Code = CodeInvalidFlag
	e.Cause = detail
	e.SuggestedFix = fmt.Sprintf("see `aim --help` or the leaf's "+
		"`--help` for valid values of --%s", flag)
	e.Alternatives = []string{
		"aim --help",
		"aim spec --format json",
	}
	return e
}

// Network wraps an HTTP transport failure from refresh.
func Network(url string, cause error) *output.Error {
	msg := fmt.Sprintf("network error fetching %s", url)
	causeMsg := ""
	if cause != nil {
		causeMsg = cause.Error()
	}
	return &output.Error{
		Code:    CodeNetwork,
		Message: msg,
		Cause:   causeMsg,
		SuggestedFix: "check network connectivity then retry with " +
			"`aim refresh --force`; `aim status` shows the last " +
			"successful fetch from the local cache",
		Alternatives: []string{
			"aim refresh --force",
			"aim status",
		},
		ExitCode: exitTransient,
	}
}

// CacheCorrupt is returned when the local cache is unreadable.
func CacheCorrupt(path string, cause error) *output.Error {
	msg := fmt.Sprintf("cache corrupt: %s", path)
	causeMsg := ""
	if cause != nil {
		causeMsg = cause.Error()
	}
	return &output.Error{
		Code:    CodeCacheCorrupt,
		Message: msg,
		Cause:   causeMsg,
		SuggestedFix: "run `aim refresh --force` to re-fetch the " +
			"upstream registry and rebuild the cache",
		Alternatives: []string{
			"aim refresh --force",
			"aim status",
		},
		ExitCode: exitGeneric,
	}
}

// SourceUnavailable is returned when the upstream returns a 5xx or the
// fetch deadline expires.
func SourceUnavailable(url string, cause error) *output.Error {
	msg := fmt.Sprintf("upstream source unavailable: %s", url)
	causeMsg := ""
	if cause != nil {
		causeMsg = cause.Error()
	}
	return &output.Error{
		Code:    CodeSourceUnavailable,
		Message: msg,
		Cause:   causeMsg,
		SuggestedFix: "check models.dev availability, then retry " +
			"with `aim refresh --force`",
		Alternatives: []string{
			"aim refresh --force",
			"aim status",
		},
		ExitCode: exitTransient,
	}
}

// CacheLocked is returned when a concurrent refresh holds the lockfile
// beyond timeout.
func CacheLocked(path string) *output.Error {
	e := output.ConflictError(fmt.Sprintf("cache locked: %s", path))
	e.Code = CodeCacheLocked
	e.Cause = "another aim refresh is in progress"
	e.SuggestedFix = "wait for the active refresh to complete and " +
		"retry; the lockfile auto-clears on process exit"
	e.Alternatives = []string{
		"aim status",
		"aim refresh --force",
	}
	return e
}

// classify inspects a generic error returned by aim's library layer and
// maps it onto the closest envelope constructor. Used by RunE wrappers
// that can't introspect deep call paths without invasive refactors.
//
// Heuristics (cheap-first):
//   - context-deadline → SourceUnavailable
//   - HTTP status mentions ("unexpected status 5xx") → SourceUnavailable
//   - "fetch" or "dial" string → Network
//   - "lock timeout" → CacheLocked
//   - everything else → nil (caller falls back to its default code)
//
// Returning nil tells the caller "use your own default", which keeps
// classify safe to chain.
func classify(url string, err error) *output.Error {
	if err == nil {
		return nil
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "context deadline exceeded"),
		strings.Contains(s, "context canceled"):
		return SourceUnavailable(url, err)
	case strings.Contains(s, "unexpected status 5"):
		return SourceUnavailable(url, err)
	case strings.Contains(s, "lock timeout"):
		return CacheLocked(url)
	case strings.Contains(s, "dial "),
		strings.Contains(s, "no such host"),
		strings.Contains(s, "connection refused"),
		strings.Contains(s, "i/o timeout"):
		return Network(url, err)
	}
	return nil
}

// FromRefreshError maps a Registry.Refresh / Cache.Refresh error onto
// the right envelope. url is the upstream URL aim was talking to.
func FromRefreshError(url string, err error) *output.Error {
	if err == nil {
		return nil
	}
	if e := classify(url, err); e != nil {
		return e
	}
	// Default: assume transport/network class.
	return Network(url, err)
}
