package conformance

import (
	"errors"
	"strings"
	"testing"

	"hop.top/aim/internal/errs"
)

// TestFactor4Errors verifies the Factor 4 contract: every error
// emits a structured envelope with a non-empty code, suggested fix,
// and alternatives. Every aim-specific code carries the same shape.
//
// Spec ref: 12-factor AI-CLI §4 — Structured Error Recovery.
// Implementation: internal/errs/errs.go + kit's WrapRunE middleware.
func TestFactor4Errors(t *testing.T) {
	primeXDGCache(t)

	// `aim show nope nope --format json` → NOT_FOUND envelope on stderr.
	root := newRoot(t)
	stdout, stderr, err := runCmd(t, root, "show", "nope", "nope", "--format", "json")
	if err == nil {
		t.Fatalf("aim show nope nope expected error; got nil\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	env := parseEnvelope(t, stdout+stderr)
	if code, _ := env["code"].(string); code != "NOT_FOUND" {
		t.Errorf("show.code = %q, want NOT_FOUND", code)
	}
	if fix, _ := env["suggested_fix"].(string); fix == "" {
		t.Errorf("show.suggested_fix is empty (spec §4.2)")
	}
	alts, _ := env["alternatives"].([]any)
	if len(alts) == 0 {
		t.Errorf("show.alternatives is empty (spec §4.2)")
	}

	// `aim query 'bogus:::syntax' --format json` → INVALID_QUERY envelope.
	root2 := newRoot(t)
	stdout2, stderr2, err := runCmd(t, root2, "query", "bogus:::syntax", "--format", "json")
	if err == nil {
		t.Fatalf("aim query bogus expected error; got nil\nstdout: %s\nstderr: %s", stdout2, stderr2)
	}
	env2 := parseEnvelope(t, stdout2+stderr2)
	if code, _ := env2["code"].(string); code != errs.CodeInvalidQuery {
		t.Errorf("query.code = %q, want %s", code, errs.CodeInvalidQuery)
	}

	// Every error code constructor in errs returns a non-empty
	// SuggestedFix. We exercise the constructors directly rather than
	// reflecting over package-level vars (the constants are codes,
	// not constructors).
	cases := map[string]struct {
		code string
		err  any
	}{
		"NotFound provider":   {"NOT_FOUND", errs.NotFound("provider", "x")},
		"NotFound model":      {"NOT_FOUND", errs.NotFound("model", "x/y")},
		"InvalidQuery":        {errs.CodeInvalidQuery, errs.InvalidQuery("e", errors.New("c"))},
		"InvalidFlag":         {errs.CodeInvalidFlag, errs.InvalidFlag("f", "v", "d")},
		"Network":             {errs.CodeNetwork, errs.Network("http://x", errors.New("c"))},
		"CacheCorrupt":        {errs.CodeCacheCorrupt, errs.CacheCorrupt("/p", errors.New("c"))},
		"SourceUnavailable":   {errs.CodeSourceUnavailable, errs.SourceUnavailable("http://x", errors.New("c"))},
		"CacheLocked":         {errs.CodeCacheLocked, errs.CacheLocked("/p")},
	}
	for name, c := range cases {
		// Each *output.Error implements error; convert via a struct reach.
		if e, ok := c.err.(interface {
			Error() string
		}); !ok || strings.TrimSpace(e.Error()) == "" {
			t.Errorf("%s: error stringer empty", name)
		}
		// Type-assert the concrete output.Error via the actual import
		// in helpers; we re-import here to access the fields.
		oe := asOutputError(c.err)
		if oe == nil {
			t.Errorf("%s: constructor returned nil", name)
			continue
		}
		if oe.Code != c.code {
			t.Errorf("%s: code = %q, want %q", name, oe.Code, c.code)
		}
		if strings.TrimSpace(oe.SuggestedFix) == "" {
			t.Errorf("%s: SuggestedFix empty (spec §4.2 — every code must carry a fix)", name)
		}
		if len(oe.Alternatives) == 0 {
			t.Errorf("%s: Alternatives empty (spec §4.2)", name)
		}
	}
}
