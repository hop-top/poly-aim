package apiversion_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hop.top/aim/internal/apiversion"
	"hop.top/aim/internal/errs"
)

func TestNegotiate_EmptyReturnsCurrent(t *testing.T) {
	got, envErr := apiversion.Negotiate("")
	require.Nil(t, envErr, "empty request should be a no-op, never an error")
	assert.Equal(t, apiversion.Current, got,
		"empty request must resolve to the current schema version")
}

func TestNegotiate_CurrentPasses(t *testing.T) {
	got, envErr := apiversion.Negotiate(apiversion.Current)
	require.Nil(t, envErr)
	assert.Equal(t, apiversion.Current, got)
}

func TestNegotiate_BelowMinErrors(t *testing.T) {
	_, envErr := apiversion.Negotiate("0.9")
	require.NotNil(t, envErr, "version below current must surface an envelope")
	assert.Equal(t, errs.CodeInvalidFlag, envErr.Code)
	assert.Equal(t, 2, envErr.ExitCode,
		"unsupported --api-version maps to exit code 2 (usage error)")
	assert.Contains(t, envErr.Cause, "0.9")
	assert.Contains(t, envErr.Cause, apiversion.Current,
		"the cause must enumerate the supported set so callers self-diagnose")
}

func TestNegotiate_NewerMajorErrors(t *testing.T) {
	_, envErr := apiversion.Negotiate("2.0")
	require.NotNil(t, envErr)
	assert.Equal(t, errs.CodeInvalidFlag, envErr.Code)
	assert.Contains(t, envErr.Cause, "2.0")
}

func TestNegotiate_NewerMinorErrors(t *testing.T) {
	// 1.1 isn't in Supported today; should fail even though it's same
	// major + newer-by-one minor.
	_, envErr := apiversion.Negotiate("1.1")
	require.NotNil(t, envErr)
	assert.Equal(t, errs.CodeInvalidFlag, envErr.Code)
}

func TestNegotiate_GarbageErrors(t *testing.T) {
	cases := []string{"foo", "v1.0", "1", "1.0.0", " 1.0 ", "1.x"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, envErr := apiversion.Negotiate(c)
			if c == " 1.0 " {
				// Negotiate trims whitespace before allowlist; this is a
				// success case.
				assert.Nil(t, envErr,
					"whitespace-padded supported versions must pass after trim")
				return
			}
			require.NotNil(t, envErr, "garbage value %q must error", c)
			assert.Equal(t, errs.CodeInvalidFlag, envErr.Code)
		})
	}
}

func TestCompatible_SameVersion(t *testing.T) {
	assert.True(t, apiversion.Compatible("1.0", "1.0"))
}

func TestCompatible_OlderMinorPasses(t *testing.T) {
	assert.True(t, apiversion.Compatible("1.0", "1.5"),
		"requested 1.0 must run under current 1.5 (additive minor bumps)")
}

func TestCompatible_NewerMinorFails(t *testing.T) {
	assert.False(t, apiversion.Compatible("1.5", "1.0"),
		"requested 1.5 must NOT run under current 1.0 — agent expects fields the current schema cannot emit")
}

func TestCompatible_DifferentMajorFails(t *testing.T) {
	assert.False(t, apiversion.Compatible("2.0", "1.0"))
	assert.False(t, apiversion.Compatible("1.0", "2.0"))
}

func TestCompatible_GarbageInputsFail(t *testing.T) {
	cases := []struct {
		req, cur string
	}{
		{"", "1.0"},
		{"1.0", ""},
		{"foo", "1.0"},
		{"1.0", "foo"},
		{"1", "1.0"},
		{"1.0.0", "1.0"},
	}
	for _, c := range cases {
		t.Run(c.req+"/"+c.cur, func(t *testing.T) {
			assert.False(t, apiversion.Compatible(c.req, c.cur))
		})
	}
}

func TestSupported_ContainsCurrent(t *testing.T) {
	// Sanity invariant — Current must always be honor-able.
	var found bool
	for _, v := range apiversion.Supported {
		if v == apiversion.Current {
			found = true
			break
		}
	}
	assert.True(t, found,
		"apiversion.Current (%q) must appear in Supported", apiversion.Current)
}
