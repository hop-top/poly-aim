package conformance

import (
	"encoding/json"
	"testing"
	"time"
)

// TestFactor11Provenance verifies the Factor 11 contract: every read
// command's `_meta` envelope carries source, fetched_at (RFC3339),
// method, and cached fields; cache hits set cached=true with a
// positive cache_age.
//
// Spec ref: 12-factor AI-CLI §11 — Provenance.
// Implementation: internal/cmd/provenance.go provenanceFromCache +
// provenanceForRefresh.
func TestFactor11Provenance(t *testing.T) {
	primeXDGCache(t)

	root := newRoot(t)
	stdout, stderr, err := runCmd(t, root, "list", "--format", "json")
	if err != nil {
		t.Fatalf("aim list failed: %v\nstderr: %s", err, stderr)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("decode list envelope: %v", err)
	}
	meta := envelopeMeta(t, env)

	// source, fetched_at, method, cached present + correctly typed.
	if src, _ := meta["source"].(string); src == "" {
		t.Errorf("_meta.source empty")
	}
	fa, _ := meta["fetched_at"].(string)
	if fa == "" {
		t.Errorf("_meta.fetched_at empty")
	} else if _, parseErr := time.Parse(time.RFC3339, fa); parseErr != nil {
		t.Errorf("_meta.fetched_at not RFC3339: %q (%v)", fa, parseErr)
	}
	if method, _ := meta["method"].(string); method == "" {
		t.Errorf("_meta.method empty")
	}
	if cached, ok := meta["cached"].(bool); !ok || !cached {
		t.Errorf("_meta.cached = %v, want true (cache primed)", meta["cached"])
	}
	if age, ok := meta["cache_age"].(float64); !ok || age <= 0 {
		t.Errorf("_meta.cache_age = %v, want > 0", meta["cache_age"])
	}

	// Check show envelope too (object payload path, not slice).
	root2 := newRoot(t)
	out2, errOut2, err := runCmd(t, root2, "show", "openai", "gpt-4o", "--format", "json")
	if err != nil {
		t.Fatalf("aim show failed: %v\nstderr: %s", err, errOut2)
	}
	var env2 map[string]any
	if err := json.Unmarshal([]byte(out2), &env2); err != nil {
		t.Fatalf("decode show envelope: %v", err)
	}
	meta2 := envelopeMeta(t, env2)
	if cached, ok := meta2["cached"].(bool); !ok || !cached {
		t.Errorf("show _meta.cached = %v, want true", meta2["cached"])
	}
}
