package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/aim/internal/errs"
)

// TestFactor12Evolution verifies the Factor 12 contract:
// --api-version negotiation rejects unsupported versions with a
// structured envelope, accepts the current version transparently, and
// the schema changelog records every shipped version.
//
// Spec ref: 12-factor AI-CLI §12 — Evolution Guarantees.
// Implementation: internal/apiversion/apiversion.go Negotiate +
// cmd/aim/main.go installAPIVersionGuard, docs/schema-changelog.md.
func TestFactor12Evolution(t *testing.T) {
	primeXDGCache(t)

	// --api-version 0.9 → AIM_INVALID_FLAG.
	root := newRoot(t)
	stdout, stderr, err := runCmd(t, root, "--api-version", "0.9", "list", "--format", "json")
	if err == nil {
		t.Fatalf("expected error for --api-version 0.9; got nil\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	env := parseEnvelope(t, stdout+stderr)
	if code, _ := env["code"].(string); code != errs.CodeInvalidFlag {
		t.Errorf("0.9 code = %q, want %s", code, errs.CodeInvalidFlag)
	}
	if msg, _ := env["message"].(string); !strings.Contains(msg, "api-version") {
		t.Errorf("0.9 message %q does not mention api-version", msg)
	}
	if cause, _ := env["cause"].(string); !strings.Contains(cause, "0.9") {
		t.Errorf("0.9 cause %q does not mention 0.9", cause)
	}
	alts, _ := env["alternatives"].([]any)
	foundSpec := false
	for _, a := range alts {
		if s, _ := a.(string); strings.Contains(s, "aim spec") {
			foundSpec = true
		}
	}
	if !foundSpec {
		t.Errorf("alternatives missing `aim spec` pointer: %v", alts)
	}

	// --api-version 1.0 → succeeds, same shape as no flag.
	root2 := newRoot(t)
	out, errOut, err := runCmd(t, root2, "--api-version", "1.0", "list", "--format", "json")
	if err != nil {
		t.Fatalf("--api-version 1.0 failed: %v\nstderr: %s", err, errOut)
	}
	if !strings.Contains(out, "\"data\"") {
		t.Errorf("--api-version 1.0 missing data envelope:\n%s", out)
	}

	// --api-version 2.0 → same error shape.
	root3 := newRoot(t)
	out2, err2, runErr := runCmd(t, root3, "--api-version", "2.0", "list", "--format", "json")
	if runErr == nil {
		t.Fatalf("expected error for --api-version 2.0; got nil")
	}
	env3 := parseEnvelope(t, out2+err2)
	if code, _ := env3["code"].(string); code != errs.CodeInvalidFlag {
		t.Errorf("2.0 code = %q, want %s", code, errs.CodeInvalidFlag)
	}

	// docs/schema-changelog.md exists and mentions 1.0.0.
	changelog := filepath.Join("..", "..", "docs", "schema-changelog.md")
	body, err := os.ReadFile(changelog)
	if err != nil {
		t.Fatalf("read schema-changelog.md: %v", err)
	}
	if !strings.Contains(string(body), "1.0.0") {
		t.Errorf("schema-changelog.md does not mention 1.0.0")
	}
}
