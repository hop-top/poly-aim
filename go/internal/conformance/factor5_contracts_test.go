package conformance

import (
	"strings"
	"testing"

	"hop.top/kit/go/console/cli"
)

// TestFactor5Contracts verifies the Factor 5 contract: every leaf
// declares its side-effect class, idempotency class, and structured
// output schema via kit's metadata accessors. Agents read these to
// route invocations safely.
//
// Spec ref: 12-factor AI-CLI §5 — Capability Contracts.
// Implementation: per-leaf cli.SetSideEffect / SetIdempotency /
// SetOutputSchema calls in internal/cmd/*.go.
func TestFactor5Contracts(t *testing.T) {
	root := newRoot(t)

	for _, c := range leafCommands(root) {
		path := "aim " + strings.Join(leafPath(c), " ")

		se, ok := cli.GetSideEffect(c)
		if !ok || se == "" {
			t.Errorf("%s: kit/side-effect missing (spec §5.1)", path)
		}

		idem, ok := cli.GetIdempotency(c)
		if !ok || idem == "" {
			t.Errorf("%s: kit/idempotent missing (spec §5.1)", path)
		}

		// output_schema_version pairs with output_schema; we check the
		// version via the JSON accessor for parity with the manifest.
		_, sv, schemaOK := cli.GetOutputSchemaJSON(c)
		if !schemaOK || sv == "" {
			t.Errorf("%s: kit/output-schema-version missing (spec §5.2)", path)
		}

		// At least one example per leaf.
		ex, ok := cli.GetExamples(c)
		if !ok || len(ex) == 0 {
			t.Errorf("%s: kit/examples missing or empty (spec §5.3)", path)
		}
	}
}
