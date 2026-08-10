package cmd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hop.top/aim"
	"hop.top/aim/internal/cmd"
	"hop.top/aim/internal/errs"
	"hop.top/kit/go/console/cli"
)

// buildRoot wires the aim subcommands onto a fresh kit Root configured
// with DisableValidate=true so e2e error tests focus on RunE output and
// don't trip the annotation validator (which is exercised elsewhere).
func buildRoot(t *testing.T) *cli.Root {
	t.Helper()
	root := cli.New(cli.Config{
		Name:            "aim",
		Version:         "0.0.0-test",
		Short:           "aim test",
		DisableValidate: true,
	})
	root.Cmd.AddCommand(
		cmd.ListCmd(root),
		cmd.ShowCmd(root),
		cmd.ProvidersCmd(root),
		cmd.RefreshCmd(root),
		cmd.QueryCmd(root),
	)
	// WrapRunE is normally called by Root.Execute. We exercise
	// Cmd.ExecuteContext directly so we can capture stderr, so wire
	// the middleware explicitly here — same call kit makes in prod.
	root.WrapRunE()
	return root
}

// fixtureServer serves the testdata/api-fixture.json payload at /api.json.
func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "api-fixture.json"))
	require.NoError(t, err, "read testdata/api-fixture.json")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
}

// primeCache populates an XDG cache directory from the fixture server
// so show/list don't try to hit models.dev during the test.
func primeCache(t *testing.T) string {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)

	srv := fixtureServer(t)
	t.Cleanup(srv.Close)

	dir := filepath.Join(xdg, "hop", "aim")
	require.NoError(t, os.MkdirAll(dir, 0o750))

	src := &aim.ModelsDevSource{URL: srv.URL}
	c := aim.NewCache(src, aim.WithCacheDir(dir))
	_, err := c.Refresh(context.Background(), true)
	require.NoError(t, err)
	return dir
}

// extractEnvelope picks the first JSON object out of combined stdout +
// stderr. The kit middleware emits the envelope on stderr followed by
// a "ERROR" formatted block; we want the structured payload.
func extractEnvelope(t *testing.T, s string) map[string]any {
	t.Helper()
	start := strings.Index(s, "{")
	require.GreaterOrEqualf(t, start, 0, "no JSON object in output: %q", s)
	// Walk forward counting braces to find the matching close.
	depth := 0
	end := -1
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
		if end > 0 {
			break
		}
	}
	require.Greater(t, end, start, "no balanced JSON object in output: %q", s)

	var env map[string]any
	require.NoError(t, json.Unmarshal([]byte(s[start:end]), &env), "envelope JSON parse")
	return env
}

// TestE2E_ShowNotFound_ProviderEnvelope — `aim show nope nope --format json`
// emits NOT_FOUND with exit 3.
func TestE2E_ShowNotFound_ProviderEnvelope(t *testing.T) {
	primeCache(t)
	root := buildRoot(t)

	buf := &strings.Builder{}
	root.Cmd.SetOut(buf)
	root.Cmd.SetErr(buf)
	root.Cmd.SetArgs([]string{"show", "nonexistent_provider", "nonexistent_model", "--format", "json"})

	err := root.Cmd.ExecuteContext(context.Background())
	require.Error(t, err, "expected non-nil error from RunE")

	env := extractEnvelope(t, buf.String())
	assert.Equal(t, "NOT_FOUND", env["code"])
	assert.Contains(t, env["message"], "provider not found")
	assert.Contains(t, env["message"], "nonexistent_provider")
	// ExitCode 3 (not-found) marshals as float64 via encoding/json.
	assert.Equal(t, float64(3), env["exit_code"])
	assert.NotEmpty(t, env["suggested_fix"])
}

// TestE2E_QueryInvalid_Envelope — `aim query 'bogus:::syntax' --format json`
// emits INVALID_QUERY with exit 2.
func TestE2E_QueryInvalid_Envelope(t *testing.T) {
	primeCache(t)
	root := buildRoot(t)

	buf := &strings.Builder{}
	root.Cmd.SetOut(buf)
	root.Cmd.SetErr(buf)
	root.Cmd.SetArgs([]string{"query", "bogus:::syntax", "--format", "json"})

	err := root.Cmd.ExecuteContext(context.Background())
	require.Error(t, err)

	env := extractEnvelope(t, buf.String())
	assert.Equal(t, "INVALID_QUERY", env["code"])
	assert.Contains(t, env["message"], "invalid query")
	assert.Equal(t, float64(2), env["exit_code"])
	assert.NotEmpty(t, env["cause"])
}

// TestE2E_ShowModelMissing — provider exists but the model does not.
func TestE2E_ShowModelMissing(t *testing.T) {
	primeCache(t)
	root := buildRoot(t)

	buf := &strings.Builder{}
	root.Cmd.SetOut(buf)
	root.Cmd.SetErr(buf)
	root.Cmd.SetArgs([]string{"show", "openai", "model-that-does-not-exist", "--format", "json"})

	err := root.Cmd.ExecuteContext(context.Background())
	require.Error(t, err)

	env := extractEnvelope(t, buf.String())
	assert.Equal(t, "NOT_FOUND", env["code"])
	assert.Contains(t, env["message"], "model not found")
}

// TestE2E_ListInvalidQuery — `aim list "bogus:::"` routes through the
// INVALID_QUERY envelope by way of list's query path.
func TestE2E_ListInvalidQuery(t *testing.T) {
	primeCache(t)
	root := buildRoot(t)

	buf := &strings.Builder{}
	root.Cmd.SetOut(buf)
	root.Cmd.SetErr(buf)
	root.Cmd.SetArgs([]string{"list", "bogus:::syntax", "--format", "json"})

	err := root.Cmd.ExecuteContext(context.Background())
	require.Error(t, err)

	env := extractEnvelope(t, buf.String())
	assert.Equal(t, errs.CodeInvalidQuery, env["code"])
}
