package conformance

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestFactor1Introspection verifies the Factor 1 contract: `aim spec`
// emits a machine-readable capability manifest including every leaf
// with the full set of agent-facing metadata fields.
//
// Spec ref: 12-factor AI-CLI §1 — Capability Introspection.
// Implementation: cmd/aim/main.go RegisterSpecCommand call +
// per-leaf cli.SetOutputSchema / SetExamples / SetNextSteps annotations.
func TestFactor1Introspection(t *testing.T) {
	root := newRoot(t)

	stdout, stderr, err := runCmd(t, root, "spec", "--format", "json")
	if err != nil {
		t.Fatalf("aim spec --format json failed: %v\nstderr: %s", err, stderr)
	}

	var manifest map[string]any
	if err := json.Unmarshal([]byte(stdout), &manifest); err != nil {
		t.Fatalf("decode manifest: %v\nstdout: %s", err, stdout)
	}

	// Top-level keys required by §1.
	for _, key := range []string{"tool", "version", "schema_version", "commands"} {
		if _, ok := manifest[key]; !ok {
			t.Errorf("manifest missing top-level key %q (spec §1.2)", key)
		}
	}
	if got, _ := manifest["tool"].(string); got != "aim" {
		t.Errorf("manifest.tool = %q, want \"aim\"", got)
	}
	if got, _ := manifest["schema_version"].(string); got != "1.0" {
		t.Errorf("manifest.schema_version = %q, want \"1.0\"", got)
	}

	cmds, _ := manifest["commands"].([]any)
	if len(cmds) < 7 {
		t.Fatalf("manifest.commands has %d entries; want >= 7 (5 adopter + spec + status)",
			len(cmds))
	}

	// Adopter leaves carry the full agent-facing surface; kit-shipped
	// spec/status are exempt from next_steps (kit chose not to wire
	// next_steps on `spec`, which we can't extend without forking kit
	// — see "factor 1 follow-up" in the test report).
	requiredAll := []string{
		"path", "short", "side_effect", "idempotent",
		"output_schema_version", "examples", "top_level_verb",
	}
	adopterLeaves := map[string]struct{}{
		"aim list":      {},
		"aim show":      {},
		"aim providers": {},
		"aim refresh":   {},
		"aim query":     {},
	}
	for i, raw := range cmds {
		c, ok := raw.(map[string]any)
		if !ok {
			t.Errorf("commands[%d] not an object: %T", i, raw)
			continue
		}
		pathSeg, _ := c["path"].([]any)
		pathStr := ""
		if len(pathSeg) > 0 {
			parts := make([]string, len(pathSeg))
			for j, p := range pathSeg {
				parts[j], _ = p.(string)
			}
			pathStr = strings.Join(parts, " ")
		}
		for _, key := range requiredAll {
			if _, ok := c[key]; !ok {
				t.Errorf("manifest.commands[%s] missing required field %q (spec §1.3)", pathStr, key)
			}
		}
		// Long is required on adopter leaves; kit-shipped `spec`
		// concatenates a single long string by builder convention.
		if _, ok := c["long"]; !ok {
			t.Errorf("manifest.commands[%s] missing long description", pathStr)
		}
		// output_schema_version pair is required on every leaf.
		if got, _ := c["output_schema_version"].(string); got == "" {
			t.Errorf("manifest.commands[%s] output_schema_version empty", pathStr)
		}
		// next_steps is required on every adopter leaf so agents
		// always get a follow-up suggestion. Kit-shipped commands are
		// exempt per the note above.
		if _, isAdopter := adopterLeaves[pathStr]; isAdopter {
			if _, ok := c["next_steps"]; !ok {
				t.Errorf("manifest.commands[%s] missing next_steps (spec §1.3, required on adopter leaves)", pathStr)
			}
		}
	}

	// `aim spec --version` returns just the schema version. The
	// version-only short-circuit emits a tiny {"schema_version": "..."}
	// payload — no commands list, no full manifest.
	root2 := newRoot(t)
	stdout2, stderr2, err := runCmd(t, root2, "spec", "--version", "--format", "json")
	if err != nil {
		t.Fatalf("aim spec --version failed: %v\nstderr: %s", err, stderr2)
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(stdout2), &probe); err != nil {
		t.Fatalf("decode --version probe: %v\nstdout: %s", err, stdout2)
	}
	if got, _ := probe["schema_version"].(string); got != "1.0" {
		t.Errorf("--version schema_version = %q, want \"1.0\"", got)
	}
	if _, hasCmds := probe["commands"]; hasCmds {
		t.Errorf("--version response leaked commands payload: %s", stdout2)
	}
}
