package errs

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hop.top/kit/go/console/output"
)

// TestNotFoundProvider — provider-flavor envelope.
func TestNotFoundProvider(t *testing.T) {
	e := NotFound("provider", "openai")
	require.NotNil(t, e)
	assert.Equal(t, output.CodeNotFound, e.Code)
	assert.Equal(t, exitGeneric, e.ExitCode)
	assert.Contains(t, e.Message, "provider not found")
	assert.Contains(t, e.Message, "openai")
	assert.NotEmpty(t, e.SuggestedFix)
	assert.Greater(t, len(e.Alternatives), 0)
	// Provider variant must mention `aim providers` somewhere.
	assert.Contains(t, e.SuggestedFix, "aim providers")
}

// TestNotFoundModel — model-flavor envelope.
func TestNotFoundModel(t *testing.T) {
	e := NotFound("model", "gpt-99")
	require.NotNil(t, e)
	assert.Equal(t, output.CodeNotFound, e.Code)
	assert.Contains(t, e.Message, "model not found")
	assert.Contains(t, e.Message, "gpt-99")
	assert.Contains(t, e.SuggestedFix, "aim list")
}

// TestInvalidQuery — wraps the underlying parser error in Cause.
func TestInvalidQuery(t *testing.T) {
	cause := errors.New("aim: unknown tag key \"bogus\"")
	e := InvalidQuery("bogus:value", cause)
	require.NotNil(t, e)
	assert.Equal(t, CodeInvalidQuery, e.Code)
	assert.Equal(t, exitUsage, e.ExitCode)
	assert.Contains(t, e.Message, "invalid query")
	assert.Contains(t, e.Cause, "bogus")
	assert.NotEmpty(t, e.SuggestedFix)
	assert.Greater(t, len(e.Alternatives), 0)
}

// TestInvalidFlag — flag-validation envelope.
func TestInvalidFlag(t *testing.T) {
	e := InvalidFlag("format", "xml", "must be one of: table, json, yaml")
	require.NotNil(t, e)
	assert.Equal(t, CodeInvalidFlag, e.Code)
	assert.Equal(t, exitUsage, e.ExitCode)
	assert.Contains(t, e.Message, "--format")
	assert.Contains(t, e.Message, "xml")
	assert.Contains(t, e.Cause, "must be one of")
	assert.NotEmpty(t, e.SuggestedFix)
}

// TestNetwork — transport-failure envelope.
func TestNetwork(t *testing.T) {
	e := Network("https://models.dev/api.json",
		errors.New("dial tcp: lookup models.dev: no such host"))
	require.NotNil(t, e)
	assert.Equal(t, CodeNetwork, e.Code)
	assert.Equal(t, exitIO, e.ExitCode)
	assert.Contains(t, e.Message, "network error")
	assert.Contains(t, e.Cause, "no such host")
	assert.Contains(t, e.SuggestedFix, "aim refresh --force")
	assert.Greater(t, len(e.Alternatives), 0)
}

// TestCacheCorrupt — cache-unreadable envelope.
func TestCacheCorrupt(t *testing.T) {
	e := CacheCorrupt("/tmp/aim/meta.json",
		errors.New("invalid character 'x' looking for beginning of value"))
	require.NotNil(t, e)
	assert.Equal(t, CodeCacheCorrupt, e.Code)
	assert.Equal(t, exitIO, e.ExitCode)
	assert.Contains(t, e.Message, "cache corrupt")
	assert.Contains(t, e.Message, "/tmp/aim/meta.json")
	assert.Contains(t, e.SuggestedFix, "aim refresh --force")
}

// TestSourceUnavailable — upstream-5xx / deadline envelope.
func TestSourceUnavailable(t *testing.T) {
	e := SourceUnavailable("https://models.dev/api.json",
		errors.New("context deadline exceeded"))
	require.NotNil(t, e)
	assert.Equal(t, CodeSourceUnavailable, e.Code)
	assert.Equal(t, exitIO, e.ExitCode)
	assert.Contains(t, e.Message, "upstream source unavailable")
	assert.Contains(t, e.Cause, "deadline")
	assert.Contains(t, e.SuggestedFix, "models.dev")
}

// TestCacheLocked — concurrent-refresh envelope.
func TestCacheLocked(t *testing.T) {
	e := CacheLocked("/tmp/aim/.lock")
	require.NotNil(t, e)
	assert.Equal(t, CodeCacheLocked, e.Code)
	assert.Equal(t, exitIO, e.ExitCode)
	assert.Contains(t, e.Message, "cache locked")
	assert.Contains(t, e.SuggestedFix, "lockfile")
}

// TestFromRefreshError_DeadlineMapsToSourceUnavailable — classify hits.
func TestFromRefreshError_DeadlineMapsToSourceUnavailable(t *testing.T) {
	e := FromRefreshError("https://models.dev/api.json",
		errors.New("Get \"...\": context deadline exceeded"))
	require.NotNil(t, e)
	assert.Equal(t, CodeSourceUnavailable, e.Code)
}

// TestFromRefreshError_5xxMapsToSourceUnavailable — server-error mapping.
func TestFromRefreshError_5xxMapsToSourceUnavailable(t *testing.T) {
	e := FromRefreshError("https://models.dev/api.json",
		errors.New("aim: fetch https://models.dev/api.json: unexpected status 503"))
	require.NotNil(t, e)
	assert.Equal(t, CodeSourceUnavailable, e.Code)
}

// TestFromRefreshError_DialFailure — DNS/transport mapping.
func TestFromRefreshError_DialFailure(t *testing.T) {
	e := FromRefreshError("https://models.dev/api.json",
		errors.New("dial tcp: lookup models.dev: no such host"))
	require.NotNil(t, e)
	assert.Equal(t, CodeNetwork, e.Code)
}

// TestFromRefreshError_LockTimeout — lockfile mapping.
func TestFromRefreshError_LockTimeout(t *testing.T) {
	e := FromRefreshError("/tmp/aim/.lock",
		errors.New("aim: acquire lock: aim: lock timeout after 5s"))
	require.NotNil(t, e)
	assert.Equal(t, CodeCacheLocked, e.Code)
}

// TestFromRefreshError_NilPassthrough — nil in, nil out.
func TestFromRefreshError_NilPassthrough(t *testing.T) {
	e := FromRefreshError("https://x", nil)
	assert.Nil(t, e)
}

// TestRoundTripJSON — RenderError emits the documented wire shape.
func TestRoundTripJSON(t *testing.T) {
	e := NotFound("provider", "openai")
	var buf bytes.Buffer
	require.NoError(t, output.RenderError(&buf, output.JSON, e))

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))

	assert.Equal(t, "NOT_FOUND", got["code"])
	assert.Contains(t, got["message"], "openai")
	assert.NotEmpty(t, got["suggested_fix"])
	alts, ok := got["alternatives"].([]any)
	require.True(t, ok)
	assert.Greater(t, len(alts), 0)
	exit, ok := got["exit_code"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(exitGeneric), exit)
}

// TestRoundTripPlain — table-mode/empty-format renders human prose
// containing the code+message and the fix line.
func TestRoundTripPlain(t *testing.T) {
	e := InvalidQuery("bogus:value", errors.New("aim: unknown tag key \"bogus\""))
	var buf bytes.Buffer
	require.NoError(t, output.RenderError(&buf, "", e))

	s := buf.String()
	assert.True(t, strings.HasPrefix(s, "INVALID_QUERY:"))
	assert.Contains(t, s, "Fix:")
	assert.Contains(t, s, "Alternative:")
}
