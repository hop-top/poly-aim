package conformance

import (
	"encoding/json"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestFactor3StructuredIO verifies the Factor 3 contract: every leaf
// emits structured envelopes (data + _meta) in JSON / YAML, table
// format keeps human columns on stdout, and empty results are
// rendered as `data: []` rather than prose.
//
// Spec ref: 12-factor AI-CLI §3 — Structured I/O.
// Implementation: kit's output.Render with WithProvenance wires the
// envelope; cmd.list returns []modelRow even when empty.
func TestFactor3StructuredIO(t *testing.T) {
	primeXDGCache(t)

	// JSON envelope on `aim list`.
	root := newRoot(t)
	stdout, stderr, err := runCmd(t, root, "list", "--format", "json")
	if err != nil {
		t.Fatalf("aim list --format json failed: %v\nstderr: %s", err, stderr)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("decode list envelope: %v\nstdout: %s", err, stdout)
	}
	if _, ok := env["_meta"]; !ok {
		t.Errorf("list JSON missing _meta envelope (spec §3.1)")
	}
	if _, ok := env["data"]; !ok {
		t.Errorf("list JSON missing data field (spec §3.1)")
	}

	// Empty result: list with an impossible filter (no model has video
	// input) returns data:[] not prose.
	root2 := newRoot(t)
	emptyOut, emptyErr, err := runCmd(t, root2, "list", "--input", "video", "--format", "json")
	if err != nil {
		t.Fatalf("aim list --input video failed: %v\nstderr: %s", err, emptyErr)
	}
	if strings.Contains(emptyOut, "No models") {
		t.Errorf("empty result leaked prose: %s", emptyOut)
	}
	var empty struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(emptyOut), &empty); err != nil {
		t.Fatalf("decode empty envelope: %v", err)
	}
	if len(empty.Data) != 0 {
		t.Errorf("data has %d rows on impossible filter; want 0", len(empty.Data))
	}

	// Table format keeps a column header on stdout (Provider).
	root3 := newRoot(t)
	tableOut, tableErr, err := runCmd(t, root3, "list", "--format", "table")
	if err != nil {
		t.Fatalf("aim list --format table failed: %v\nstderr: %s", err, tableErr)
	}
	if !strings.Contains(tableOut, "Provider") {
		t.Errorf("table stdout missing 'Provider' column header:\n%s", tableOut)
	}
	if strings.Contains(tableOut, "_meta") || strings.Contains(tableOut, "\"data\":") {
		t.Errorf("table stdout leaked envelope JSON:\n%s", tableOut)
	}

	// YAML format is parseable and carries the same envelope keys.
	root4 := newRoot(t)
	yamlOut, yamlErr, err := runCmd(t, root4, "list", "--format", "yaml")
	if err != nil {
		t.Fatalf("aim list --format yaml failed: %v\nstderr: %s", err, yamlErr)
	}
	var yamlEnv map[string]any
	if err := yaml.Unmarshal([]byte(yamlOut), &yamlEnv); err != nil {
		t.Fatalf("decode YAML envelope: %v\nstdout: %s", err, yamlOut)
	}
	if _, ok := yamlEnv["_meta"]; !ok {
		t.Errorf("YAML envelope missing _meta:\n%s", yamlOut)
	}
	if _, ok := yamlEnv["data"]; !ok {
		t.Errorf("YAML envelope missing data:\n%s", yamlOut)
	}
}
