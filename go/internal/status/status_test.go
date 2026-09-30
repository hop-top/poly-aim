package status

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"hop.top/aim"
	"hop.top/kit/go/console/cli"
)

// redirectCache points the aim cache resolver at a tmp dir for the test
// run. xdg.RawCacheDir reads $XDG_CACHE_HOME, so a single t.Setenv is
// enough — provided we created the "hop/aim" subtree inside it.
func redirectCache(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", root)
	return filepath.Join(root, "hop", "aim")
}

func TestCacheProviderFirstRun(t *testing.T) {
	cacheDir := redirectCache(t)
	// Note: do NOT create cacheDir or meta.json — this is the
	// first-run state.

	sec, err := Cache()(context.Background())
	if err != nil {
		t.Fatalf("Cache provider returned error: %v", err)
	}
	if sec.Title != "cache" {
		t.Fatalf("section title = %q, want %q", sec.Title, "cache")
	}
	if sec.Status != cli.StatusOK {
		t.Fatalf("status = %q, want %q", sec.Status, cli.StatusOK)
	}

	data, ok := sec.Data.(CacheData)
	if !ok {
		t.Fatalf("section.Data type = %T, want CacheData", sec.Data)
	}
	if data.Dir != cacheDir {
		t.Fatalf("dir = %q, want %q", data.Dir, cacheDir)
	}
	if data.LastFetch != "never" {
		t.Fatalf("last_fetch = %q, want %q (first run)", data.LastFetch, "never")
	}
	if data.PayloadSize != "unknown" {
		t.Fatalf("payload_size = %q, want %q", data.PayloadSize, "unknown")
	}
	if data.StaleOnError != "enabled" {
		t.Fatalf("stale_on_error = %q, want %q", data.StaleOnError, "enabled")
	}
	if data.TTL != (24 * time.Hour).String() {
		t.Fatalf("ttl = %q, want %q", data.TTL, (24 * time.Hour).String())
	}
}

func TestCacheProviderWithMeta(t *testing.T) {
	cacheDir := redirectCache(t)
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}

	lastFetch := time.Now().Add(-2 * time.Hour).UTC()
	meta := metaOnDisk{
		LastFetch:  lastFetch,
		ETag:       "abcdef1234567890longvalue",
		TTLSeconds: int64((6 * time.Hour).Seconds()),
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, cacheFileMeta), metaBytes, 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	payload := []byte(`{"openai":{"id":"openai","name":"OpenAI","models":{}}}`)
	if err := os.WriteFile(filepath.Join(cacheDir, cacheFilePayload), payload, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	sec, err := Cache()(context.Background())
	if err != nil {
		t.Fatalf("Cache provider returned error: %v", err)
	}
	data := sec.Data.(CacheData)

	if data.LastFetch != lastFetch.Format(time.RFC3339) {
		t.Fatalf("last_fetch = %q, want RFC3339 of %v", data.LastFetch, lastFetch)
	}
	if data.Age == "" {
		t.Fatalf("age unset; want non-empty when last_fetch present")
	}
	if data.ETag != "abcdef123456" {
		t.Fatalf("etag = %q, want truncated 12-char %q", data.ETag, "abcdef123456")
	}
	if data.PayloadBytes != int64(len(payload)) {
		t.Fatalf("payload_bytes = %d, want %d", data.PayloadBytes, len(payload))
	}
	if data.PayloadSize == "unknown" || data.PayloadSize == "" {
		t.Fatalf("payload_size = %q, want human-readable", data.PayloadSize)
	}
	if data.TTL != (6 * time.Hour).String() {
		t.Fatalf("ttl = %q, want %q (overridden by meta)", data.TTL, (6 * time.Hour).String())
	}
}

func TestSourceProvider(t *testing.T) {
	sec, err := Source()(context.Background())
	if err != nil {
		t.Fatalf("Source provider returned error: %v", err)
	}
	if sec.Title != "source" {
		t.Fatalf("title = %q", sec.Title)
	}
	data, ok := sec.Data.(SourceData)
	if !ok {
		t.Fatalf("data type = %T, want SourceData", sec.Data)
	}
	if data.URL != aim.DefaultSourceURL {
		t.Fatalf("url = %q, want %q", data.URL, aim.DefaultSourceURL)
	}
	if data.RetrievalMethod != "http_get" {
		t.Fatalf("retrieval_method = %q", data.RetrievalMethod)
	}
	if data.Timeout != (30 * time.Second).String() {
		t.Fatalf("timeout = %q", data.Timeout)
	}
}

func TestIdentityProvider(t *testing.T) {
	sec, err := Identity("1.2.3")(context.Background())
	if err != nil {
		t.Fatalf("Identity provider returned error: %v", err)
	}
	data, ok := sec.Data.(IdentityData)
	if !ok {
		t.Fatalf("data type = %T, want IdentityData", sec.Data)
	}
	if data.AIMVersion != "1.2.3" {
		t.Fatalf("aim_version = %q, want %q", data.AIMVersion, "1.2.3")
	}
	// kit_version comes from runtime/debug. Under `go test` BuildInfo
	// reports Deps from go.mod, so we expect non-empty (either a
	// version like "v0.4.0-alpha.6" or "(devel)" / "unknown").
	if data.KitVersion == "" {
		t.Fatalf("kit_version empty; want non-empty (BuildInfo or fallback)")
	}
}

func TestIdentityProviderEmptyVersionDefaults(t *testing.T) {
	sec, _ := Identity("")(context.Background())
	data := sec.Data.(IdentityData)
	if data.AIMVersion != "dev" {
		t.Fatalf("aim_version with empty input = %q, want %q", data.AIMVersion, "dev")
	}
}

func TestPathsProvider(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))

	sec, err := Paths()(context.Background())
	if err != nil {
		t.Fatalf("Paths provider returned error: %v", err)
	}
	data, ok := sec.Data.(PathsData)
	if !ok {
		t.Fatalf("data type = %T, want PathsData", sec.Data)
	}
	if data.ConfigDir == "" {
		t.Fatalf("config_dir empty")
	}
	if !strings.HasSuffix(data.CacheDir, filepath.Join("hop", "aim")) {
		t.Fatalf("cache_dir = %q, want suffix hop/aim", data.CacheDir)
	}
	if data.DataDir == "" {
		t.Fatalf("data_dir empty")
	}
}

func TestEnvironmentProviderAllowAndDeny(t *testing.T) {
	// Wipe inherited env that would pollute the assertion. We only
	// keep the deliberately-set ones below.
	for _, k := range []string{"AIM_TEST_X", "AIM_TOKEN", "XDG_CONFIG_HOME", "SECRET_KEY", "GOWORK", "HOME"} {
		t.Setenv(k, "")
	}
	t.Setenv("AIM_TEST_X", "foo")
	t.Setenv("AIM_TOKEN", "supersecret")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	t.Setenv("SECRET_KEY", "must-not-leak")
	t.Setenv("GOWORK", "")

	sec, err := Environment()(context.Background())
	if err != nil {
		t.Fatalf("Environment provider returned error: %v", err)
	}
	env, ok := sec.Data.(map[string]string)
	if !ok {
		t.Fatalf("data type = %T, want map[string]string", sec.Data)
	}

	if got := env["AIM_TEST_X"]; got != "foo" {
		t.Fatalf("AIM_TEST_X = %q, want %q", got, "foo")
	}
	if _, present := env["SECRET_KEY"]; present {
		t.Fatalf("SECRET_KEY leaked into env section (value=%q)", env["SECRET_KEY"])
	}
	if _, present := env["HOME"]; present {
		t.Fatalf("HOME leaked into env section")
	}
	// AIM_TOKEN matches the AIM_* allowlist but the name contains
	// "TOKEN" → must redact, not omit.
	if got := env["AIM_TOKEN"]; got != "[redacted]" {
		t.Fatalf("AIM_TOKEN = %q, want %q", got, "[redacted]")
	}
	if got := env["XDG_CONFIG_HOME"]; got != "/tmp/xdg" {
		t.Fatalf("XDG_CONFIG_HOME = %q, want %q", got, "/tmp/xdg")
	}
	if got := env["GOWORK"]; got != "(unset)" {
		t.Fatalf("GOWORK with empty value = %q, want %q", got, "(unset)")
	}
}

func TestEnvironmentProviderEmptyReportsEmptyStatus(t *testing.T) {
	// Clear every allow-listed name so the section is genuinely empty.
	for _, e := range os.Environ() {
		eq := strings.IndexByte(e, '=')
		if eq < 0 {
			continue
		}
		k := e[:eq]
		if strings.HasPrefix(k, "AIM_") ||
			strings.HasPrefix(k, "XDG_") ||
			k == "NO_COLOR" ||
			k == "GOWORK" {
			t.Setenv(k, "")
			// t.Setenv keeps the var present (with empty value) for
			// the test lifetime; that's actually what we want to
			// assert against — empty values should still surface as
			// "(unset)" not produce an empty section. Use Unsetenv
			// instead to truly drop it.
			_ = os.Unsetenv(k)
		}
	}
	sec, err := Environment()(context.Background())
	if err != nil {
		t.Fatalf("Environment provider returned error: %v", err)
	}
	if sec.Status != cli.StatusEmpty {
		t.Fatalf("status = %q, want %q (no env present)", sec.Status, cli.StatusEmpty)
	}
}

// TestRegisterEndToEnd builds a kit Root with EnforceValidate=false
// (no annotation requirements), mounts WithStatus, registers every
// aim provider, then runs the status command via ExecuteContext.
// Verifies that:
//   - All aim sections appear in the rendered output.
//   - No SECRET_KEY-style key leaks (defence-in-depth on
//     kit's redactor + our allowlist).
func TestRegisterEndToEnd(t *testing.T) {
	redirectCache(t)
	t.Setenv("AIM_TEST_FOO", "barvalue")
	t.Setenv("SECRET_KEY", "must-not-appear-anywhere")

	root := cli.New(cli.Config{
		Name:            "aim",
		Version:         "0.0.0-test",
		Short:           "aim test",
		DisableValidate: true,
	}, cli.WithStatus(cli.StatusConfig{}))

	Register(root, "0.0.0-test")

	out := &strings.Builder{}
	root.Cmd.SetOut(out)
	root.Cmd.SetErr(out)
	root.Cmd.SetArgs([]string{"status", "--format", "json", "--show-sensitive"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("status ExecuteContext: %v", err)
	}

	rendered := out.String()
	for _, title := range []string{"cache", "source", "identity", "paths", "environment"} {
		if !strings.Contains(rendered, title) {
			t.Fatalf("rendered output missing section %q\n----\n%s\n----", title, rendered)
		}
	}
	if strings.Contains(rendered, "must-not-appear-anywhere") {
		t.Fatalf("SECRET_KEY value leaked into rendered output:\n%s", rendered)
	}
	if !strings.Contains(rendered, "barvalue") {
		t.Fatalf("AIM_TEST_FOO value missing from rendered output:\n%s", rendered)
	}
	if !strings.Contains(rendered, aim.DefaultSourceURL) {
		t.Fatalf("DefaultSourceURL missing from rendered output:\n%s", rendered)
	}
}

// TestRegisterAllProvidersAttached confirms every named provider is
// present on the Root after Register so future audits can iterate.
func TestRegisterAllProvidersAttached(t *testing.T) {
	root := cli.New(cli.Config{
		Name:            "aim",
		Short:           "aim",
		DisableValidate: true,
	}, cli.WithStatus(cli.StatusConfig{}))
	Register(root, "test")

	out := &strings.Builder{}
	root.Cmd.SetOut(out)
	root.Cmd.SetErr(out)
	root.Cmd.SetArgs([]string{"status", "--format", "json"})
	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute: %v", err)
	}

	for _, want := range []string{"cache", "source", "identity", "paths"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("section %q missing from output:\n%s", want, out.String())
		}
	}
}

func findChild(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Use == name || c.Name() == name {
			return c
		}
	}
	return nil
}

var _ = findChild // keep helper available for future debugging
