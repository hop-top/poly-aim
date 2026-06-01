package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/aim"
	"hop.top/kit/go/console/cli"
)

// primeCache writes a fixture payload + meta.json into
// $XDG_CACHE_HOME/hop/aim so leaves that hit the cache layer see a
// pre-populated cache without touching the network.
func primeCache(t *testing.T, payload []byte) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", root)
	dir := filepath.Join(root, "hop", "aim")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "models-dev.json"), payload, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	meta := struct {
		LastFetch  time.Time `json:"last_fetch"`
		ETag       string    `json:"etag,omitempty"`
		TTLSeconds int64     `json:"ttl_seconds"`
	}{
		LastFetch:  time.Now().Add(-1 * time.Hour).UTC(),
		ETag:       "W/\"envelope-test\"",
		TTLSeconds: int64((24 * time.Hour).Seconds()),
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), metaBytes, 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	return dir
}

// fixturePayload reads testdata/api-fixture.json from the repo root.
func fixturePayload(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "api-fixture.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

// testRoot returns a kit Root with validation disabled.
func testRoot(t *testing.T) *cli.Root {
	t.Helper()
	return cli.New(cli.Config{
		Name:            "aim",
		Version:         "0.0.0-test",
		Short:           "aim envelope test",
		DisableValidate: true,
	})
}

// TestEnvelope_List_JSON_TopLevelKeys asserts that `aim list --format
// json` emits a {data, _meta} envelope and that _meta carries the
// provenance fields required by factor 11.
func TestEnvelope_List_JSON_TopLevelKeys(t *testing.T) {
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

	var env struct {
		Data []map[string]any `json:"data"`
		Meta map[string]any   `json:"_meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if len(env.Data) == 0 {
		t.Fatalf("data is empty; want at least one row\nraw: %s", stdout.String())
	}
	if env.Meta == nil {
		t.Fatalf("_meta missing; envelope shape broken\nraw: %s", stdout.String())
	}
	if got, _ := env.Meta["source"].(string); got != aim.DefaultSourceURL {
		t.Fatalf("_meta.source = %q, want %q", got, aim.DefaultSourceURL)
	}
	fa, ok := env.Meta["fetched_at"].(string)
	if !ok || fa == "" {
		t.Fatalf("_meta.fetched_at missing or non-string\nmeta: %#v", env.Meta)
	}
	if _, perr := time.Parse(time.RFC3339, fa); perr != nil {
		t.Fatalf("_meta.fetched_at not RFC3339: %q (%v)", fa, perr)
	}
	if cached, ok := env.Meta["cached"].(bool); !ok || !cached {
		t.Fatalf("_meta.cached = %v, want true (cache was primed)", env.Meta["cached"])
	}
}

// TestEnvelope_Show_JSON_CachedTrue asserts `aim show` emits an
// object payload wrapped in the envelope and reports cached=true.
func TestEnvelope_Show_JSON_CachedTrue(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(ShowCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"show", "openai", "gpt-4o", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute show: %v\nstderr: %s", err, stderr.String())
	}

	var env struct {
		Data map[string]any `json:"data"`
		Meta map[string]any `json:"_meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if env.Data == nil {
		t.Fatalf("data missing; want object payload\nraw: %s", stdout.String())
	}
	if id, _ := env.Data["id"].(string); id != "gpt-4o" {
		t.Fatalf("data.id = %q, want gpt-4o", id)
	}
	if cached, ok := env.Meta["cached"].(bool); !ok || !cached {
		t.Fatalf("_meta.cached = %v, want true", env.Meta["cached"])
	}
}

// TestEnvelope_Providers_JSON_EnvelopeShape asserts the providers
// command wires the envelope correctly.
func TestEnvelope_Providers_JSON_EnvelopeShape(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(ProvidersCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"providers", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute providers: %v\nstderr: %s", err, stderr.String())
	}

	var env struct {
		Data []map[string]any `json:"data"`
		Meta map[string]any   `json:"_meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if len(env.Data) == 0 {
		t.Fatalf("data is empty; want at least one provider")
	}
	if env.Meta == nil {
		t.Fatalf("_meta missing")
	}
}

// TestEnvelope_List_Table_NoEnvelopeLeak asserts table mode keeps
// stdout free of envelope JSON keys.
func TestEnvelope_List_Table_NoEnvelopeLeak(t *testing.T) {
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

	out := stdout.String()
	if strings.Contains(out, "_meta") {
		t.Fatalf("table stdout leaked _meta envelope:\n%s", out)
	}
	if strings.Contains(out, "\"data\":") {
		t.Fatalf("table stdout leaked data envelope:\n%s", out)
	}
	if !strings.Contains(out, "Provider") {
		t.Fatalf("table stdout missing column header (Provider):\n%s", out)
	}
}

// TestEnvelope_List_JSON_EmptyResults asserts that an empty result set
// renders as a structured `{data: [], _meta: {...}}` envelope and not
// as prose like "No models found."
func TestEnvelope_List_JSON_EmptyResults(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(ListCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	// Filter for a modality not present in the fixture (no model has
	// video input) — guaranteed empty result.
	root.Cmd.SetArgs([]string{"list", "--input", "video", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute list: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	if strings.Contains(out, "No models found") {
		t.Fatalf("empty-result prose leaked into JSON:\n%s", out)
	}

	var env struct {
		Data []map[string]any `json:"data"`
		Meta map[string]any   `json:"_meta"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, out)
	}
	if env.Data == nil {
		t.Fatalf("data is nil; want empty slice payload (structured empty)")
	}
	if len(env.Data) != 0 {
		t.Fatalf("data has %d rows; want 0 (filter is impossible)", len(env.Data))
	}
	if env.Meta == nil {
		t.Fatalf("_meta missing on empty result")
	}
}

// TestProvenanceForRefresh covers the refresh envelope construction
// path independently of the cobra command, so the wire shape is
// verified without needing to plumb a custom source URL into the
// refresh leaf.
func TestProvenanceForRefresh(t *testing.T) {
	m := provenanceForRefresh(aim.DefaultSourceURL)
	if m.Source != aim.DefaultSourceURL {
		t.Errorf("Source = %q, want %q", m.Source, aim.DefaultSourceURL)
	}
	if m.Method != "http_get" {
		t.Errorf("Method = %q, want http_get", m.Method)
	}
	if m.Cached {
		t.Errorf("Cached = true; want false (refresh is a live fetch)")
	}
	if m.FetchedAt.IsZero() {
		t.Errorf("FetchedAt zero; want time.Now")
	}
}
