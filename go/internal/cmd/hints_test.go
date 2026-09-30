package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// TestHintsRegistered_PerLeaf asserts every aim leaf wires at least
// two next-step hints into the shared HintSet so agents always have a
// follow-up to chain.
func TestHintsRegistered_PerLeaf(t *testing.T) {
	root := testRoot(t)
	root.Cmd.AddCommand(
		ListCmd(root),
		ShowCmd(root),
		ProvidersCmd(root),
		RefreshCmd(root),
		QueryCmd(root),
	)

	leaves := []string{"list", "show", "providers", "refresh", "query"}
	for _, leaf := range leaves {
		t.Run(leaf, func(t *testing.T) {
			hints := root.Hints.Lookup(leaf)
			if len(hints) < 2 {
				t.Fatalf("leaf %q has %d hint(s); want >= 2",
					leaf, len(hints))
			}
			for i, h := range hints {
				if strings.TrimSpace(h.Message) == "" {
					t.Errorf("leaf %q hint[%d].Message is empty", leaf, i)
				}
			}
		})
	}
}

// TestHints_NoEmissionWhenDisabled confirms the wiring honours
// `--no-hints` by toggling the viper key and re-asserting
// output.HintsEnabled.
func TestHints_NoEmissionWhenDisabled(t *testing.T) {
	root := testRoot(t)
	root.Cmd.AddCommand(ListCmd(root))

	if !output.HintsEnabled(root.Viper) {
		t.Fatalf("HintsEnabled defaults to false; expected true at construction")
	}

	root.Viper.Set("no-hints", true)
	if output.HintsEnabled(root.Viper) {
		t.Fatalf("HintsEnabled true after --no-hints=true; want false")
	}
	root.Viper.Set("no-hints", false)

	root.Viper.Set("quiet", true)
	if output.HintsEnabled(root.Viper) {
		t.Fatalf("HintsEnabled true after --quiet=true; want false")
	}
	root.Viper.Set("quiet", false)

	root.Viper.Set("hints.enabled", false)
	if output.HintsEnabled(root.Viper) {
		t.Fatalf("HintsEnabled true after hints.enabled=false; want false")
	}
}

// TestHints_NotInJSONEnvelope asserts hints are never spliced into the
// JSON envelope on stdout. Kit's contract emits hints only to stderr in
// Table+TTY mode; JSON consumers see {data, _meta} and nothing else.
func TestHints_NotInJSONEnvelope(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(ListCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"list", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute list: %v\nstderr: %s", err, stderr.String())
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if _, ok := env["hints"]; ok {
		t.Fatalf("envelope leaked top-level `hints` key:\n%s", stdout.String())
	}
	// Confirm the envelope still has the canonical {data, _meta} shape.
	if _, ok := env["data"]; !ok {
		t.Fatalf("envelope missing `data`:\n%s", stdout.String())
	}
	if _, ok := env["_meta"]; !ok {
		t.Fatalf("envelope missing `_meta`:\n%s", stdout.String())
	}
}

// TestHints_StderrSuppressedForJSON asserts the PostRunE emitter is a
// no-op for JSON output. Kit's RenderHints suppresses non-Table format
// at the renderer level; verify the wiring respects that contract by
// driving the leaf through the cobra Execute path and inspecting
// stderr for the muted "→ " sigil RenderHints would write.
func TestHints_StderrSuppressedForJSON(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(ListCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"list", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute list: %v\nstderr: %s", err, stderr.String())
	}

	if strings.Contains(stderr.String(), "→ ") {
		t.Fatalf("stderr contains hint sigil in JSON mode:\n%s",
			stderr.String())
	}
}

// TestHints_StderrSuppressedForNonTTY exercises the Table-format path
// with a bytes.Buffer stderr (not a TTY). Kit's RenderHints must
// suppress emission when the writer isn't an *os.File on a terminal.
func TestHints_StderrSuppressedForNonTTY(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(ListCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"list", "--format", "table"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute list: %v\nstderr: %s", err, stderr.String())
	}

	if strings.Contains(stderr.String(), "→ ") {
		t.Fatalf("stderr contains hint sigil with non-TTY writer:\n%s",
			stderr.String())
	}
}

// TestHints_NoHintsFlag_SuppressesViaWiring asserts that wiring
// `--no-hints=true` causes emitHints to skip RenderHints. We can't
// observe TTY output in unit tests, so we drive the helper directly
// with a viper carrying the flag and confirm HintsEnabled flips.
func TestHints_NoHintsFlag_SuppressesViaWiring(t *testing.T) {
	root := testRoot(t)
	root.Cmd.AddCommand(ListCmd(root))

	if got := output.HintsEnabled(root.Viper); !got {
		t.Fatalf("baseline HintsEnabled = %v; want true", got)
	}
	root.Viper.Set("no-hints", true)
	if got := output.HintsEnabled(root.Viper); got {
		t.Fatalf("HintsEnabled with --no-hints = %v; want false", got)
	}
}

// TestRootHints_DefaultRegistry asserts cli.New always provisions a
// non-nil HintSet on the Root so adopters can register without
// guarding for nil. Regression coverage in case kit changes its
// initialisation order.
func TestRootHints_DefaultRegistry(t *testing.T) {
	root := cli.New(cli.Config{
		Name:            "aim-hints-fixture",
		Version:         "0.0.0",
		Short:           "fixture",
		DisableValidate: true,
	})
	if root.Hints == nil {
		t.Fatalf("root.Hints is nil; cli.New must initialise the registry")
	}
	registerHints(root, "fixture",
		output.Hint{Message: "smoke"},
	)
	hints := root.Hints.Lookup("fixture")
	if len(hints) != 1 || hints[0].Message != "smoke" {
		t.Fatalf("registerHints did not round-trip; got %#v", hints)
	}
}
