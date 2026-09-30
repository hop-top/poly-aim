package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"hop.top/kit/go/console/cli"
)

// TestList_PositionalArg_EmitsDeprecationWarning asserts that supplying
// a positional argument to `aim list` still functions but lands a
// warning in envelope.warnings[] under --format json.
func TestList_PositionalArg_EmitsDeprecationWarning(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(ListCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"list", "gpt", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute list: %v\nstderr: %s", err, stderr.String())
	}

	var env struct {
		Data     []map[string]any `json:"data"`
		Meta     map[string]any   `json:"_meta"`
		Warnings []string         `json:"warnings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if len(env.Warnings) == 0 {
		t.Fatalf("warnings missing; expected deprecation notice\nraw: %s", stdout.String())
	}
	if !strings.Contains(env.Warnings[0], "deprecated") {
		t.Fatalf("warning missing 'deprecated': %q", env.Warnings[0])
	}
	if env.Meta == nil {
		t.Fatalf("_meta missing")
	}
	if env.Data == nil {
		t.Fatalf("data missing; positional should still produce results")
	}
}

// TestList_FlagsOnly_NoWarning asserts that the flag-driven form does
// NOT trigger a deprecation warning.
func TestList_FlagsOnly_NoWarning(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(ListCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"list", "--provider", "openai", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute list: %v\nstderr: %s", err, stderr.String())
	}

	var env struct {
		Data     []map[string]any `json:"data"`
		Meta     map[string]any   `json:"_meta"`
		Warnings []string         `json:"warnings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if len(env.Warnings) != 0 {
		t.Fatalf("warnings should be empty: %v", env.Warnings)
	}
}

// TestList_PositionalArg_TableMode_WarningOnStderr asserts that table
// mode emits the deprecation warning to stderr (not stdout) so the
// payload table stays clean.
func TestList_PositionalArg_TableMode_WarningOnStderr(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(ListCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"list", "gpt", "--format", "table"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute list: %v\nstderr: %s", err, stderr.String())
	}

	if !strings.Contains(stderr.String(), "deprecated") {
		t.Fatalf("stderr missing deprecation: %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "deprecated") {
		t.Fatalf("stdout leaked deprecation: %q", stdout.String())
	}
}

// TestShow_FlagForm_Works asserts `aim show --provider X --model Y`
// returns the same payload as the positional form, no warning.
func TestShow_FlagForm_Works(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(ShowCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"show", "--provider", "openai", "--model", "gpt-4o", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute show: %v\nstderr: %s", err, stderr.String())
	}

	var env struct {
		Data     map[string]any `json:"data"`
		Meta     map[string]any `json:"_meta"`
		Warnings []string       `json:"warnings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if env.Data == nil {
		t.Fatalf("data missing under flag form")
	}
	if id, _ := env.Data["id"].(string); id != "gpt-4o" {
		t.Fatalf("data.id = %q, want gpt-4o", id)
	}
	if len(env.Warnings) != 0 {
		t.Fatalf("warnings should be empty for flag-only form: %v", env.Warnings)
	}
}

// TestShow_BothForms_EmitsWarning asserts that supplying both
// positional + flag values triggers a warning telling the agent to
// prefer the flag form.
func TestShow_BothForms_EmitsWarning(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(ShowCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"show", "openai", "gpt-4o", "--provider", "openai", "--model", "gpt-4o", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute show: %v\nstderr: %s", err, stderr.String())
	}

	var env struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if len(env.Warnings) == 0 {
		t.Fatalf("expected warning when both forms supplied")
	}
	if !strings.Contains(env.Warnings[0], "positional") {
		t.Fatalf("warning should mention positional precedence: %q", env.Warnings[0])
	}
}

// TestQuery_Explain_ReturnsAST asserts `aim query --explain` parses the
// expression and returns the parsed AST without consulting the cache.
func TestQuery_Explain_ReturnsAST(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(QueryCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"query", "--explain", "provider:openai tool_call:true", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute query --explain: %v\nstderr: %s", err, stderr.String())
	}

	var env struct {
		Data map[string]any `json:"data"`
		Meta map[string]any `json:"_meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if env.Data == nil {
		t.Fatalf("data missing\nraw: %s", stdout.String())
	}
	if got, _ := env.Data["expr_input"].(string); got != "provider:openai tool_call:true" {
		t.Fatalf("expr_input = %q, want exact echo", got)
	}
	filt, ok := env.Data["filter"].(map[string]any)
	if !ok {
		t.Fatalf("filter missing or wrong type: %#v", env.Data["filter"])
	}
	if filt["provider"] != "openai" {
		t.Fatalf("filter.provider = %v, want openai", filt["provider"])
	}
	if filt["tool_call"] != true {
		t.Fatalf("filter.tool_call = %v, want true", filt["tool_call"])
	}
}

// TestQuery_Explain_InvalidExpr asserts that invalid input still flows
// through the INVALID_QUERY error envelope.
func TestQuery_Explain_InvalidExpr(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(QueryCmd(root))
	// Wire WrapRunE so the error envelope renders on stderr (mirrors
	// production setup).
	root.WrapRunE()
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"query", "--explain", "bogus_key:value", "--format", "json"})

	err := root.Cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatalf("expected error for invalid expr\nstdout: %s\nstderr: %s",
			stdout.String(), stderr.String())
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "INVALID_QUERY") {
		t.Fatalf("expected INVALID_QUERY in output\ngot: %s", combined)
	}
}

// Compile-time guard that ListCmd / ShowCmd / QueryCmd still take a
// *cli.Root — protects against accidental constructor signature drift.
var _ = func(r *cli.Root) {
	_ = ListCmd(r)
	_ = ShowCmd(r)
	_ = QueryCmd(r)
}
