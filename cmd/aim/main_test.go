package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	"hop.top/kit/go/console/output"
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
	hardenCommandGroups(root.Cmd)
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
	assert.Equal(t, 2, envelope.ExitCode)
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

// TestExitCode_ResolvesEnvelopeCodes — the process exit code mirrors
// the envelope's classified code; bare errors fall back to general (1).
func TestExitCode_ResolvesEnvelopeCodes(t *testing.T) {
	assert.Equal(t, 0, exitCode(nil))
	assert.Equal(t, 3, exitCode(errs.NotFound("provider", "x")))
	assert.Equal(t, 2, exitCode(errs.InvalidQuery("e", errors.New("c"))))
	assert.Equal(t, 2, exitCode(errs.InvalidFlag("f", "v", "d")))
	assert.Equal(t, 6, exitCode(errs.Network("http://x", errors.New("c"))))
	assert.Equal(t, 6, exitCode(errs.SourceUnavailable("http://x", errors.New("c"))))
	assert.Equal(t, 4, exitCode(errs.CacheLocked("/p")))
	assert.Equal(t, 1, exitCode(errs.CacheCorrupt("/p", errors.New("c"))))
	assert.Equal(t, 1, exitCode(errors.New("bare")))
}

// Compile-time guard: cobra import is used by ExecuteContext flow.
var _ = cobra.Command{}

// TestUnknownCommand_IsUsageError guards the cobra-hardening pass:
// `aim nosuchcommand` used to print help and exit 0, which tells an agent
// its invocation succeeded. Cobra only arg-validates runnable commands,
// so a bare group node silently swallows the unmatched positional.
// Args validators run during cobra's argument matching, before kit's
// RunE middleware is reached, so the failure surfaces as cobra's plain
// "Error: <message>" line rather than a rendered JSON envelope. What
// matters for the bug is that it is an error at all, that it carries
// CodeUsage/ExitCode 2, and that the message names the bad argument.
func TestUnknownCommand_IsUsageError(t *testing.T) {
	root := newTestRoot(t)
	_, stderr, runErr := runRoot(t, root, "nosuchcommand")
	require.Error(t, runErr,
		"an unknown command must not exit 0 after printing help")

	var envErr *output.Error
	require.ErrorAs(t, runErr, &envErr,
		"the failure must carry a structured output.Error")
	assert.Equal(t, output.CodeUsage, envErr.Code)
	assert.Equal(t, 2, envErr.ExitCode, "usage errors exit 2")
	assert.Contains(t, envErr.Message, "nosuchcommand",
		"the message must name the offending argument")
	assert.Contains(t, envErr.Message, "--help",
		"guidance must ride in Message; SuggestedFix alone never reaches stderr")

	assert.Equal(t, 2, exitCode(runErr),
		"the resolved process exit code must be 2")
	assert.Contains(t, stderr, "nosuchcommand",
		"the offending argument must reach the user on stderr")
}

// TestUnknownSubcommand_IsUsageError covers nested group nodes, not just
// the root. `status` is runnable, so cobra's own unknown-subcommand check
// fires first; either way the invocation must fail rather than exit 0.
func TestUnknownSubcommand_IsUsageError(t *testing.T) {
	root := newTestRoot(t)
	_, stderr, runErr := runRoot(t, root, "status", "nosuchsubcommand")
	require.Error(t, runErr,
		"an unknown subcommand under a group must not exit 0")
	assert.Contains(t, stderr, "nosuchsubcommand",
		"the offending argument must reach the user on stderr")

	// Whatever produced it, the resolved exit code must be nonzero.
	assert.NotEqual(t, 0, exitCode(runErr))
}

// TestBareRoot_StaysOnHelpPath asserts the hardening did not break the
// help path: `aim` with no arguments must still render help and exit 0.
func TestBareRoot_StaysOnHelpPath(t *testing.T) {
	root := newTestRoot(t)
	_, _, runErr := runRoot(t, root)
	require.NoError(t, runErr,
		"bare invocation must stay on the help path, not become a usage error")
}

// TestKnownCommandsStillDispatch is the counterweight: an over-eager
// Args validator would reject legitimate positional arguments. `show`
// and `query` both take them.
func TestKnownCommandsStillDispatch(t *testing.T) {
	root := newTestRoot(t)
	_, _, runErr := runRoot(t, root, "status", "--format", "json")
	require.NoError(t, runErr, "known leaf must still dispatch")
}

// TestVersionMatchesReleaseManifest pins the compiled-in version to the
// release-please manifest. The binary previously reported a hardcoded
// v0.1.0 while the manifest said 0.1.0-alpha.2, so `aim --version` told
// agents something no release had ever produced.
//
// There is no ldflags injection point — the Go entry in publish.yml is
// mirror-only and builds no binary — so the constant carries an
// x-release-please-version annotation and is rewritten by the release PR.
// This test is what proves the annotation is still wired up.
func TestVersionMatchesReleaseManifest(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", ".release-please-manifest.json"))
	require.NoError(t, err, "release-please manifest must be readable")

	var manifest map[string]string
	require.NoError(t, json.Unmarshal(raw, &manifest))

	want, ok := manifest["."]
	require.True(t, ok, `manifest must carry a "." entry for the Go module`)
	assert.Equal(t, want, aimVersion,
		"aimVersion drifted from the release manifest; the "+
			"x-release-please-version annotation on the constant and the "+
			"extra-files entry in release-please-config.json keep these in sync")
}
