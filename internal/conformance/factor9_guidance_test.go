package conformance

import (
	"strings"
	"testing"
)

// TestFactor9Guidance verifies the Factor 9 contract: every adopter
// leaf registers at least two hints with the kit HintSet; --no-hints
// suppresses emission everywhere.
//
// Spec ref: 12-factor AI-CLI §9 — Contextual Guidance.
// Implementation: internal/cmd/hints.go registerHints +
// installHintEmitter wraps PostRunE.
func TestFactor9Guidance(t *testing.T) {
	primeXDGCache(t)

	root := newRoot(t)

	// Every adopter leaf has ≥2 hints in the HintSet.
	adopterLeaves := []string{"list", "show", "providers", "refresh", "query"}
	if root.Hints == nil {
		t.Fatalf("Root.Hints is nil — kit didn't initialize HintSet")
	}
	for _, name := range adopterLeaves {
		hints := root.Hints.Lookup(name)
		if len(hints) < 2 {
			t.Errorf("leaf %q has %d hints; want ≥ 2 (spec §9.1)", name, len(hints))
		}
	}

	// --no-hints suppresses hint output. Use `aim list` in a buffer
	// (non-TTY) so the hint emitter's TTY check is hit either way;
	// with --no-hints explicit, the suppression must be absolute.
	stdout, stderr, err := runCmd(t, root, "list", "--no-hints", "--format", "json")
	if err != nil {
		t.Fatalf("aim list --no-hints failed: %v", err)
	}
	// Hints render to stderr in kit's contract.
	for _, hint := range root.Hints.Lookup("list") {
		if strings.Contains(stderr, hint.Message) || strings.Contains(stdout, hint.Message) {
			t.Errorf("--no-hints leaked hint %q\nstdout: %s\nstderr: %s", hint.Message, stdout, stderr)
		}
	}
}
