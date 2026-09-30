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

	"hop.top/kit/go/console/cli"
)

// dryRunRoot returns a Root with kit's --dry-run flag wired in. We need
// the persistent flag set so cli.IsDryRun reads true when the test
// supplies --dry-run via SetArgs.
func dryRunRoot(t *testing.T) *cli.Root {
	t.Helper()
	root := cli.New(cli.Config{
		Name:            "aim",
		Version:         "0.0.0-test",
		Short:           "aim dry-run test",
		DisableValidate: true,
	})
	return root
}

// dirSnapshot captures the file names + mtimes under dir so a test can
// detect any disk mutation. Returns (size, mtime) per file path.
func dirSnapshot(t *testing.T, dir string) map[string]time.Time {
	t.Helper()
	snap := make(map[string]time.Time)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return snap
		}
		t.Fatalf("readdir %s: %v", dir, err)
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatalf("stat %s: %v", e.Name(), err)
		}
		snap[e.Name()] = info.ModTime()
	}
	return snap
}

// writeFreshCacheMeta primes a cache directory with a meta.json whose
// LastFetch is age old relative to ttl. payload is the model fixture
// bytes (we just need a present file).
func writeFreshCacheMeta(t *testing.T, dir string, age, ttl time.Duration, payload []byte) {
	t.Helper()
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
		LastFetch:  time.Now().Add(-age).UTC(),
		ETag:       "W/\"dry-run-test\"",
		TTLSeconds: int64(ttl.Seconds()),
	}
	b, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), b, 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
}

// TestRefreshDryRunSkip — within TTL, no --force, dry-run reports
// would_skip and makes no network or disk changes.
func TestRefreshDryRunSkip(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	dir := filepath.Join(xdg, "hop", "aim")
	writeFreshCacheMeta(t, dir, 1*time.Hour, 24*time.Hour, fixturePayload(t))
	before := dirSnapshot(t, dir)

	root := dryRunRoot(t)
	root.Cmd.AddCommand(RefreshCmd(root))

	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"refresh", "--dry-run", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute refresh --dry-run: %v\nstderr: %s", err, stderr.String())
	}

	var env struct {
		Data map[string]any `json:"data"`
		Meta map[string]any `json:"_meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if env.Data["status"] != "would_skip" {
		t.Fatalf("status = %v, want would_skip\nraw: %s", env.Data["status"], stdout.String())
	}
	if env.Data["reason"] != "ttl_remaining" {
		t.Fatalf("reason = %v, want ttl_remaining", env.Data["reason"])
	}
	if env.Data["would_skip_due_to_ttl"] != true {
		t.Fatalf("would_skip_due_to_ttl = %v, want true", env.Data["would_skip_due_to_ttl"])
	}
	if env.Data["current_etag"] != "W/\"dry-run-test\"" {
		t.Fatalf("current_etag = %v", env.Data["current_etag"])
	}

	// No disk mutation.
	after := dirSnapshot(t, dir)
	for name, m := range before {
		if after[name] != m {
			t.Fatalf("file %s mtime changed: before=%s after=%s", name, m, after[name])
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			t.Fatalf("dry-run created new file: %s", name)
		}
	}
}

// TestRefreshDryRunForce — within TTL but --force flips the decision
// to would_refresh with reason=force.
func TestRefreshDryRunForce(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	dir := filepath.Join(xdg, "hop", "aim")
	writeFreshCacheMeta(t, dir, 1*time.Hour, 24*time.Hour, fixturePayload(t))
	before := dirSnapshot(t, dir)

	root := dryRunRoot(t)
	root.Cmd.AddCommand(RefreshCmd(root))

	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"refresh", "--dry-run", "--force", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute refresh --dry-run --force: %v\nstderr: %s", err, stderr.String())
	}

	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if env.Data["status"] != "would_refresh" {
		t.Fatalf("status = %v, want would_refresh", env.Data["status"])
	}
	if env.Data["reason"] != "force" {
		t.Fatalf("reason = %v, want force", env.Data["reason"])
	}
	if env.Data["would_skip_due_to_ttl"] != false {
		t.Fatalf("would_skip_due_to_ttl = %v, want false", env.Data["would_skip_due_to_ttl"])
	}

	// No disk mutation.
	after := dirSnapshot(t, dir)
	for name, m := range before {
		if after[name] != m {
			t.Fatalf("file %s mtime changed: before=%s after=%s", name, m, after[name])
		}
	}
}

// TestRefreshDryRunStale — past TTL, no --force, dry-run reports
// would_refresh with reason=ttl_expired.
func TestRefreshDryRunStale(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	dir := filepath.Join(xdg, "hop", "aim")
	// Cache is 25h old; TTL is 24h ⇒ expired.
	writeFreshCacheMeta(t, dir, 25*time.Hour, 24*time.Hour, fixturePayload(t))
	before := dirSnapshot(t, dir)

	root := dryRunRoot(t)
	root.Cmd.AddCommand(RefreshCmd(root))

	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"refresh", "--dry-run", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute refresh --dry-run: %v\nstderr: %s", err, stderr.String())
	}

	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if env.Data["status"] != "would_refresh" {
		t.Fatalf("status = %v, want would_refresh", env.Data["status"])
	}
	if env.Data["reason"] != "ttl_expired" {
		t.Fatalf("reason = %v, want ttl_expired", env.Data["reason"])
	}

	after := dirSnapshot(t, dir)
	for name, m := range before {
		if after[name] != m {
			t.Fatalf("file %s mtime changed: before=%s after=%s", name, m, after[name])
		}
	}
}

// TestRefreshDryRunNoPriorFetch — empty cache dir, dry-run reports
// would_refresh with reason=no_prior_fetch.
func TestRefreshDryRunNoPriorFetch(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)

	root := dryRunRoot(t)
	root.Cmd.AddCommand(RefreshCmd(root))

	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"refresh", "--dry-run", "--format", "json"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute refresh --dry-run: %v\nstderr: %s", err, stderr.String())
	}

	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw: %s", err, stdout.String())
	}
	if env.Data["status"] != "would_refresh" {
		t.Fatalf("status = %v, want would_refresh", env.Data["status"])
	}
	if env.Data["reason"] != "no_prior_fetch" {
		t.Fatalf("reason = %v, want no_prior_fetch", env.Data["reason"])
	}

	// No directory should have been created — the dry-run must not
	// initialise the cache.
	if _, err := os.Stat(filepath.Join(xdg, "hop", "aim", "meta.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created meta.json with empty cache: %v", err)
	}
}

// TestRefreshDryRunOfflineNoNetwork — dry-run must succeed without a
// network endpoint. We rely on the test running offline (no DNS, no
// HTTP) by virtue of never wiring a real Source URL and by asserting
// the existing fixture cache stays untouched.
func TestRefreshDryRunOfflineNoNetwork(t *testing.T) {
	// Point the default cache at a fresh XDG that exists but contains
	// nothing. The dry-run runs without any HTTP call possible because
	// runRefreshDryRun builds the registry but never invokes Refresh.
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)

	root := dryRunRoot(t)
	root.Cmd.AddCommand(RefreshCmd(root))

	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"refresh", "--dry-run", "--format", "json"})

	// Cancelled context: a real refresh would fail; dry-run must not
	// care because it never touches the network.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := root.Cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute refresh --dry-run with cancelled ctx: %v\nstderr: %s",
			err, stderr.String())
	}

	// Verify the wire shape — proves we got the preview not a fetch.
	if !strings.Contains(stdout.String(), "would_fetch_url") {
		t.Fatalf("preview missing would_fetch_url:\n%s", stdout.String())
	}
}
