package conformance

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestFactor2Intent verifies the Factor 2 contract: command intent is
// expressed via verbs + flags, deprecated free-text positionals warn
// rather than silently coerce, and no leaf accepts unbounded args.
//
// Spec ref: 12-factor AI-CLI §2 — Intent Clarity.
// Implementation: cmd.ListCmd MaximumNArgs(1) + warnings, cmd.ShowCmd
// dual-form Args with flag aliases, every leaf bounded.
func TestFactor2Intent(t *testing.T) {
	primeXDGCache(t)

	// `aim list foo --format json` emits a deprecation warning in the
	// envelope.warnings[] slice.
	root := newRoot(t)
	stdout, stderr, err := runCmd(t, root, "list", "foo", "--format", "json")
	if err != nil {
		t.Fatalf("aim list foo failed: %v\nstderr: %s", err, stderr)
	}
	var env struct {
		Warnings []string       `json:"warnings"`
		Meta     map[string]any `json:"_meta"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("decode envelope: %v\nstdout: %s", err, stdout)
	}
	if len(env.Warnings) == 0 {
		t.Fatalf("expected warnings[] on positional list; got %s", stdout)
	}
	foundDeprecation := false
	for _, w := range env.Warnings {
		if strings.Contains(strings.ToLower(w), "deprecated") {
			foundDeprecation = true
			break
		}
	}
	if !foundDeprecation {
		t.Errorf("warnings[] did not mention deprecation: %v", env.Warnings)
	}

	// `aim list --format json` (no positional) emits NO warning.
	root2 := newRoot(t)
	stdout2, stderr2, err := runCmd(t, root2, "list", "--format", "json")
	if err != nil {
		t.Fatalf("aim list --format json failed: %v\nstderr: %s", err, stderr2)
	}
	var env2 struct {
		Warnings []string `json:"warnings"`
	}
	_ = json.Unmarshal([]byte(stdout2), &env2)
	if len(env2.Warnings) != 0 {
		t.Errorf("warnings emitted on clean list: %v", env2.Warnings)
	}

	// `aim show --provider X --model Y --format json` works.
	root3 := newRoot(t)
	stdout3, stderr3, err := runCmd(t, root3, "show", "--provider", "openai", "--model", "gpt-4o", "--format", "json")
	if err != nil {
		t.Fatalf("aim show --provider/--model failed: %v\nstderr: %s", err, stderr3)
	}
	var env3 struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout3), &env3); err != nil {
		t.Fatalf("decode show envelope: %v\nstdout: %s", err, stdout3)
	}
	if id, _ := env3.Data["id"].(string); id != "gpt-4o" {
		t.Errorf("flag-form show.data.id = %q, want gpt-4o", id)
	}

	// `aim show openai gpt-4o --format json` (positional) also works,
	// no deprecation warning when only positional form is supplied.
	root4 := newRoot(t)
	stdout4, stderr4, err := runCmd(t, root4, "show", "openai", "gpt-4o", "--format", "json")
	if err != nil {
		t.Fatalf("aim show positional failed: %v\nstderr: %s", err, stderr4)
	}
	var env4 struct {
		Data     map[string]any `json:"data"`
		Warnings []string       `json:"warnings"`
	}
	// envelope path: when no warnings, kit's WithProvenance wraps
	// only {data, _meta}; warnings field is absent. The decode is
	// tolerant of either shape.
	_ = json.Unmarshal([]byte(stdout4), &env4)
	if id, _ := env4.Data["id"].(string); id != "gpt-4o" {
		// Show without warnings uses kit's WithProvenance directly,
		// which wraps with key "data" — accept both shapes.
		var alt map[string]any
		if err := json.Unmarshal([]byte(stdout4), &alt); err == nil {
			if d, ok := alt["data"].(map[string]any); ok {
				if did, _ := d["id"].(string); did == "gpt-4o" {
					id = did
				}
			}
		}
		if id != "gpt-4o" {
			t.Errorf("positional show.data.id = %q, want gpt-4o\nstdout: %s", id, stdout4)
		}
	}
	if len(env4.Warnings) != 0 {
		t.Errorf("positional show emitted warnings: %v", env4.Warnings)
	}

	// Sanity: every leaf has bounded args (no cobra.ArbitraryArgs).
	// ArbitraryArgs is a function pointer; cobra exposes the Args
	// field as a PositionalArgs func — we compare via the function
	// pointer identity by name, since the cobra package does not
	// export a sentinel for ArbitraryArgs.
	for _, c := range leafCommands(root) {
		// Reflect by invoking the validator with 1000 args. Bounded
		// validators (MaximumNArgs, NoArgs, ExactArgs) reject; only
		// ArbitraryArgs accepts. nil Args also accepts everything.
		if c.Args == nil {
			t.Errorf("leaf %q has nil Args validator (unbounded by default; spec §2.3)",
				strings.Join(append([]string{"aim"}, leafPath(c)...), " "))
			continue
		}
		manyArgs := make([]string, 1000)
		for i := range manyArgs {
			manyArgs[i] = "x"
		}
		if err := c.Args(c, manyArgs); err == nil {
			// Only cobra.ArbitraryArgs (and nil) accept 1000 args.
			t.Errorf("leaf %q accepts unbounded args (rule out cobra.ArbitraryArgs) — spec §2.3",
				strings.Join(append([]string{"aim"}, leafPath(c)...), " "))
		}
	}
}

// leafPath returns the cobra Use stem path from root for c.
func leafPath(c *cobra.Command) []string {
	var out []string
	for n := c; n != nil && n.HasParent(); n = n.Parent() {
		out = append([]string{n.Name()}, out...)
	}
	return out
}
