package conformance

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestFactor8State verifies the Factor 8 contract: `aim status` exposes
// every runtime-relevant subsystem; sensitive env keys never leak;
// allowlist enforcement keeps unrelated env vars out of the section.
//
// Spec ref: 12-factor AI-CLI §8 — State Transparency.
// Implementation: cli.WithStatus + internal/status/* providers in
// cmd/aim/main.go.
func TestFactor8State(t *testing.T) {
	primeXDGCache(t)

	// Drive the env scenario: set one safe key, one deny-pattern key,
	// and assert HOME is not in the allowlist (omitted entirely from
	// the aim-environment section).
	t.Setenv("AIM_TEST_X", "bar")
	t.Setenv("AIM_TEST_SECRET", "supersecret-value")

	root := newRoot(t)
	stdout, stderr, err := runCmd(t, root, "status", "--format", "json")
	if err != nil {
		t.Fatalf("aim status failed: %v\nstderr: %s", err, stderr)
	}

	var out struct {
		Sections []struct {
			Title string         `json:"title"`
			Data  map[string]any `json:"data"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode status: %v\nstdout: %s", err, stdout)
	}

	// Required sections — kit-shipped + aim-shipped.
	required := map[string]bool{
		// aim-shipped (priority 1000+):
		"cache":          false,
		"source":         false,
		"identity":       false,
		"paths":          false,
		"environment":    false,
		"source-breaker": false,
		// kit-shipped (priority 100-600):
		"profile":          false,
		"env":              false,
		"workspace":        false,
		"auth":             false,
		"effective-config": false,
		"kit-annotations":  false,
	}
	var envSection map[string]any
	for _, s := range out.Sections {
		if _, want := required[s.Title]; want {
			required[s.Title] = true
		}
		if s.Title == "environment" {
			envSection = s.Data
		}
	}
	for name, found := range required {
		if !found {
			t.Errorf("status missing required section %q (spec §8.1)", name)
		}
	}

	// AIM_TEST_X is in the allowlist (AIM_*) and is not redacted; the
	// raw value must be present.
	if envSection == nil {
		t.Fatalf("environment section had no data block")
	}
	if got, _ := envSection["AIM_TEST_X"].(string); got != "bar" {
		t.Errorf("environment.AIM_TEST_X = %q, want \"bar\"", got)
	}

	// HOME is NOT in the allowlist and must be entirely omitted from
	// the aim-environment section.
	if _, present := envSection["HOME"]; present {
		t.Errorf("environment leaked HOME (not in allowlist; spec §8.2)")
	}

	// The raw value of AIM_TEST_SECRET must NEVER appear in the
	// environment payload (the deny-pattern hides the value via
	// [redacted]; the impl keeps the key with the redacted value).
	envBytes, _ := json.Marshal(envSection)
	if strings.Contains(string(envBytes), "supersecret-value") {
		t.Errorf("environment leaked AIM_TEST_SECRET raw value:\n%s", envBytes)
	}
}
