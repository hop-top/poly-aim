package conformance

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"hop.top/aim/internal/errs"
	"hop.top/kit/go/console/output"
)

// TestFactor4ExitCodes locks aim's exit-code taxonomy end-to-end:
// constructor mapping, envelope wire value, and the per-leaf
// kit/exit-codes manifest contract emitted by `aim spec`.
//
// Taxonomy: 0 success, 1 general, 2 usage, 3 not-found, 4 conflict,
// 5 permission, 6 transient/retryable, 64 rate-limited.
//
// Spec ref: 12-factor AI-CLI §4 — Structured Error Recovery.
// Implementation: internal/errs/errs.go constructors +
// internal/cmd/exitcodes.go annotations + cmd/aim/main.go exitCode.
func TestFactor4ExitCodes(t *testing.T) {
	// 1. Constructor table — every errs constructor lands its code on
	// the right taxonomy slot. Literal numbers on purpose: this table
	// is the contract, not a mirror of the implementation constants.
	cases := []struct {
		name     string
		env      *output.Error
		wantCode string
		wantExit int
	}{
		{"NotFound", errs.NotFound("provider", "x"), "NOT_FOUND", 3},
		{"InvalidQuery", errs.InvalidQuery("e", errors.New("c")), "INVALID_QUERY", 2},
		{"InvalidFlag", errs.InvalidFlag("f", "v", "d"), "AIM_INVALID_FLAG", 2},
		{"Network", errs.Network("http://x", errors.New("c")), "AIM_NETWORK", 6},
		{"SourceUnavailable", errs.SourceUnavailable("http://x", errors.New("c")), "AIM_SOURCE_UNAVAILABLE", 6},
		{"CacheCorrupt", errs.CacheCorrupt("/p", errors.New("c")), "AIM_CACHE_CORRUPT", 1},
		{"CacheLocked", errs.CacheLocked("/p"), "AIM_CACHE_LOCKED", 4},
	}
	for _, c := range cases {
		if c.env == nil {
			t.Errorf("%s: constructor returned nil", c.name)
			continue
		}
		if c.env.Code != c.wantCode {
			t.Errorf("%s: code = %q, want %q", c.name, c.env.Code, c.wantCode)
		}
		if c.env.ExitCode != c.wantExit {
			t.Errorf("%s: exit_code = %d, want %d", c.name, c.env.ExitCode, c.wantExit)
		}
	}

	// 2. Wire shape — envelopes emitted by real invocations carry the
	// taxonomy exit code.
	primeXDGCache(t)

	root := newRoot(t)
	_, stderr, err := runCmd(t, root, "show", "nope", "nope", "--format", "json")
	if err == nil {
		t.Fatalf("aim show nope nope expected error; got nil")
	}
	env := parseEnvelope(t, stderr)
	if got, _ := env["exit_code"].(float64); got != 3 {
		t.Errorf("show.exit_code = %v, want 3 (not-found)", env["exit_code"])
	}

	root2 := newRoot(t)
	_, stderr2, err := runCmd(t, root2, "query", "bogus:::syntax", "--format", "json")
	if err == nil {
		t.Fatalf("aim query bogus expected error; got nil")
	}
	env2 := parseEnvelope(t, stderr2)
	if got, _ := env2["exit_code"].(float64); got != 2 {
		t.Errorf("query.exit_code = %v, want 2 (usage)", env2["exit_code"])
	}

	root3 := newRoot(t)
	_, stderr3, err := runCmd(t, root3,
		"--api-version", "0.9", "list", "--format", "json")
	if err == nil {
		t.Fatalf("aim --api-version 0.9 expected error; got nil")
	}
	env3 := parseEnvelope(t, stderr3)
	if got, _ := env3["exit_code"].(float64); got != 2 {
		t.Errorf("api-version.exit_code = %v, want 2 (usage)", env3["exit_code"])
	}

	// 3. Manifest — every adopter leaf publishes its exit-code classes
	// via the kit/exit-codes annotation; show additionally declares
	// NOT_FOUND for lookup misses.
	root4 := newRoot(t)
	stdout4, stderr4, err := runCmd(t, root4, "spec", "--format", "json")
	if err != nil {
		t.Fatalf("aim spec --format json failed: %v\nstderr: %s", err, stderr4)
	}
	var manifest struct {
		Commands []struct {
			Path      []string `json:"path"`
			ExitCodes []string `json:"exit_codes"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(stdout4), &manifest); err != nil {
		t.Fatalf("decode manifest: %v\nstdout: %s", err, stdout4)
	}
	declared := map[string][]string{}
	for _, mc := range manifest.Commands {
		declared[strings.Join(mc.Path, " ")] = mc.ExitCodes
	}
	wantCommon := []string{"OK", "GENERIC", "USAGE", "CONFLICT", "TRANSIENT"}
	for _, leaf := range []string{
		"aim list", "aim show", "aim providers", "aim refresh", "aim query",
	} {
		codes, ok := declared[leaf]
		if !ok || len(codes) == 0 {
			t.Errorf("manifest.commands[%s] missing exit_codes (kit/exit-codes annotation)", leaf)
			continue
		}
		for _, want := range wantCommon {
			if !containsString(codes, want) {
				t.Errorf("manifest.commands[%s] exit_codes = %v, missing %q", leaf, codes, want)
			}
		}
	}
	if codes := declared["aim show"]; !containsString(codes, "NOT_FOUND") {
		t.Errorf("manifest.commands[aim show] exit_codes = %v, missing NOT_FOUND", codes)
	}
}

// containsString reports whether s appears in xs.
func containsString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
