package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hop.top/aim/internal/apiversion"
	"hop.top/aim/internal/cmd"
	"hop.top/aim/internal/errs"
	"hop.top/aim/internal/status"
	speccli "hop.top/kit/go/ai/toolspec/cli"
	"hop.top/kit/go/console/cli"
)

// newTestRoot mirrors main() but disables validation + skips fang's
// Execute so tests can capture stdout/stderr cleanly. WrapRunE is
// invoked explicitly so the api-version guard chains correctly.
func newTestRoot(t *testing.T) *cli.Root {
	t.Helper()
	root := cli.New(cli.Config{
		Name:            "aim",
		Version:         aimVersion,
		Short:           "AI model registry — query models.dev",
		Accent:          "#7DFFB3",
		DisableValidate: true,
	}, cli.WithStatus(cli.StatusConfig{
		ExtraEnvKeys: []string{"AIM_*", "XDG_*", "NO_COLOR", "GOWORK"},
	}))
	root.Cmd.Long = "aim test harness"
	status.Register(root, aimVersion)
	root.Cmd.AddCommand(
		cmd.ListCmd(root),
		cmd.ShowCmd(root),
		cmd.ProvidersCmd(root),
		cmd.RefreshCmd(root),
		cmd.QueryCmd(root),
	)
	require.NoError(t, speccli.RegisterSpecCommand(root, apiversion.Current))
	installAPIVersionGuard(root)
	root.WrapRunE()
	return root
}

// runRoot drives root.Cmd through cobra's ExecuteContext with the
// supplied args. Returns the captured stdout / stderr / exec error so
// tests can assert on the envelope shape.
func runRoot(t *testing.T, root *cli.Root, args ...string) (string, string, error) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.Cmd.SetOut(stdout)
	root.Cmd.SetErr(stderr)
	root.Cmd.SetArgs(args)
	err := root.Cmd.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}

func TestAPIVersion_UnsupportedBelow_EmitsEnvelope(t *testing.T) {
	root := newTestRoot(t)
	_, stderr, runErr := runRoot(t, root,
		"--api-version", "0.9", "list", "--format", "json")
	require.Error(t, runErr,
		"unsupported --api-version must surface a RunE error")

	envelope := extractErrorEnvelope(t, stderr)
	assert.Equal(t, errs.CodeInvalidFlag, envelope.Code)
	assert.Contains(t, envelope.Cause, "0.9")
	assert.Contains(t, envelope.Cause, "1.0",
		"the cause must enumerate the Supported set")
	assert.Equal(t, 64, envelope.ExitCode)
}

func TestAPIVersion_UnsupportedAbove_EmitsEnvelope(t *testing.T) {
	root := newTestRoot(t)
	_, stderr, runErr := runRoot(t, root,
		"--api-version", "2.0", "list", "--format", "json")
	require.Error(t, runErr)

	envelope := extractErrorEnvelope(t, stderr)
	assert.Equal(t, errs.CodeInvalidFlag, envelope.Code)
	assert.Contains(t, envelope.Cause, "2.0")
}

func TestAPIVersion_CurrentSucceeds(t *testing.T) {
	// Pin status as the no-network leaf — list would require a primed
	// cache. Status runs against the live process state.
	root := newTestRoot(t)
	_, _, runErr := runRoot(t, root,
		"--api-version", "1.0", "status", "--format", "json")
	require.NoError(t, runErr,
		"requesting Current must be identical to the default behaviour")
}

func TestAPIVersion_EmptyIsNoOp(t *testing.T) {
	root := newTestRoot(t)
	_, _, runErr := runRoot(t, root, "status", "--format", "json")
	require.NoError(t, runErr,
		"omitting --api-version must be a no-op")
}

func TestAPIVersion_GuardRunsOnSpecLeaf(t *testing.T) {
	// Factor 12 also gates the kit-shipped spec leaf — agents probing
	// the manifest with a bad --api-version must see the same envelope
	// shape as agents probing adopter leaves.
	root := newTestRoot(t)
	_, stderr, runErr := runRoot(t, root,
		"--api-version", "0.5", "spec", "--format", "json")
	require.Error(t, runErr,
		"spec leaf must honour the api-version guard too")
	envelope := extractErrorEnvelope(t, stderr)
	assert.Equal(t, errs.CodeInvalidFlag, envelope.Code)
}

// extractErrorEnvelope finds the JSON object in stderr that decodes
// into an output.Error-shaped struct. Kit's WrapRunE writes the
// envelope to stderr in JSON mode; fang's styled banner is non-JSON
// and gets skipped by the brace counter.
func extractErrorEnvelope(t *testing.T, stderr string) errorEnvelope {
	t.Helper()
	// Find first '{' and matching '}' on the brace balance.
	start := strings.Index(stderr, "{")
	require.GreaterOrEqual(t, start, 0,
		"expected JSON envelope on stderr; got %q", stderr)
	depth := 0
	end := -1
	for i := start; i < len(stderr); i++ {
		switch stderr[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	require.GreaterOrEqual(t, end, 0,
		"could not find balanced JSON envelope in stderr: %q", stderr)
	var env errorEnvelope
	require.NoError(t, json.Unmarshal([]byte(stderr[start:end+1]), &env))
	return env
}

// errorEnvelope mirrors hop.top/kit/go/console/output.Error JSON tags.
// Duplicated locally so tests don't need to import the kit type just
// for assertion.
type errorEnvelope struct {
	Code         string   `json:"code"`
	Message      string   `json:"message"`
	Cause        string   `json:"cause"`
	SuggestedFix string   `json:"suggested_fix"`
	Alternatives []string `json:"alternatives"`
	ExitCode     int      `json:"exit_code"`
}

// Compile-time guard: cobra import is used by ExecuteContext flow.
var _ = cobra.Command{}
