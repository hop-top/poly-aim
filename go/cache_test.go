package aim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// staticSource is a simple Source that returns a fixed provider map and counts calls.
type staticSource struct {
	mu       sync.Mutex
	calls    int
	data     map[string]*Provider
	err      error
	etag     string
}

func newStaticSource(data map[string]*Provider) *staticSource {
	return &staticSource{data: data}
}

func (s *staticSource) Fetch(ctx context.Context) (map[string]*Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

// FetchWithETag implements the etagSource interface used by Cache.fetchWithETag.
func (s *staticSource) FetchWithETag(ctx context.Context, etag string) (map[string]*Provider, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return nil, "", s.err
	}
	if etag != "" && etag == s.etag {
		// 304 — return nil, same etag
		return nil, s.etag, nil
	}
	return s.data, s.etag, nil
}

func (s *staticSource) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// singleProviderData returns a minimal provider map for testing.
func singleProviderData() map[string]*Provider {
	return map[string]*Provider{
		"acme": {
			ID:   "acme",
			Name: "Acme",
			Models: map[string]*Model{
				"m1": {ID: "m1", Name: "Model 1", Provider: "acme"},
			},
		},
	}
}

// writeMeta writes a cacheMeta JSON to dir/meta.json.
func writeMeta(t *testing.T, dir string, meta cacheMeta) {
	t.Helper()
	b, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, fileMeta), b, 0o600))
}

// writePayload writes provider data JSON to dir/models-dev.json.
func writePayload(t *testing.T, dir string, data map[string]*Provider) {
	t.Helper()
	b, err := json.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePayload), b, 0o600))
}

// TestCache_TTL_FreshNotRefetched verifies that a fresh cache avoids calling the source.
func TestCache_TTL_FreshNotRefetched(t *testing.T) {
	dir := t.TempDir()
	data := singleProviderData()
	src := newStaticSource(data)

	// Pre-populate cache as fresh.
	writePayload(t, dir, data)
	writeMeta(t, dir, cacheMeta{
		LastFetch:  time.Now().UTC(),
		TTLSeconds: 3600,
	})

	c := NewCache(src, WithCacheDir(dir), WithTTL(time.Hour))
	providers, err := c.Refresh(context.Background(), false)
	require.NoError(t, err)
	assert.NotNil(t, providers)
	assert.Equal(t, 0, src.Calls(), "source should not be called when cache is fresh")
}

// TestCache_TTL_ExpiredTriggersRefresh verifies expired cache calls source.
func TestCache_TTL_ExpiredTriggersRefresh(t *testing.T) {
	dir := t.TempDir()
	data := singleProviderData()
	src := newStaticSource(data)

	// Pre-populate cache as expired.
	writePayload(t, dir, data)
	writeMeta(t, dir, cacheMeta{
		LastFetch:  time.Now().UTC().Add(-2 * time.Hour),
		TTLSeconds: 3600,
	})

	c := NewCache(src, WithCacheDir(dir), WithTTL(time.Hour))
	providers, err := c.Refresh(context.Background(), false)
	require.NoError(t, err)
	assert.NotNil(t, providers)
	assert.Equal(t, 1, src.Calls(), "source should be called when cache is expired")
}

// TestCache_ETag_304ServesCache verifies that a 304 response uses cached data.
func TestCache_ETag_304ServesCache(t *testing.T) {
	dir := t.TempDir()
	data := singleProviderData()

	// Source with etag set — will return nil data (304) when etag matches.
	src := &staticSource{data: data, etag: "abc123"}

	// Pre-populate cache as expired so it will re-fetch.
	writePayload(t, dir, data)
	writeMeta(t, dir, cacheMeta{
		LastFetch:  time.Now().UTC().Add(-2 * time.Hour),
		ETag:       "abc123",
		TTLSeconds: 3600,
	})

	c := NewCache(src, WithCacheDir(dir), WithTTL(time.Hour))
	providers, err := c.Refresh(context.Background(), false)
	require.NoError(t, err)
	require.NotNil(t, providers)
	// Data should be the cached data (from disk).
	assert.NotNil(t, providers["acme"])
}

// TestCache_Lockfile_Concurrent verifies concurrent Refresh calls don't corrupt.
func TestCache_Lockfile_Concurrent(t *testing.T) {
	dir := t.TempDir()
	data := singleProviderData()
	src := newStaticSource(data)

	c := NewCache(src, WithCacheDir(dir), WithTTL(time.Hour))

	const goroutines = 5
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	results := make([]map[string]*Provider, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = c.Refresh(context.Background(), true)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "goroutine %d failed", i)
		require.NotNil(t, results[i], "goroutine %d got nil result", i)
	}

	// Verify cache file is valid JSON.
	b, err := os.ReadFile(filepath.Join(dir, filePayload))
	require.NoError(t, err)
	var out map[string]*Provider
	require.NoError(t, json.Unmarshal(b, &out))
}

// TestCache_AtomicWrite verifies no partial files exist on success.
func TestCache_AtomicWrite_NoPartialFiles(t *testing.T) {
	dir := t.TempDir()
	data := singleProviderData()
	src := newStaticSource(data)

	c := NewCache(src, WithCacheDir(dir), WithTTL(time.Hour))
	_, err := c.Refresh(context.Background(), true)
	require.NoError(t, err)

	// No .tmp- files should remain.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, len(e.Name()) > 4 && e.Name()[:5] == ".tmp-",
			"unexpected temp file: %s", e.Name())
	}

	// Payload and meta must exist.
	_, err = os.Stat(filepath.Join(dir, filePayload))
	assert.NoError(t, err, "payload file must exist")
	_, err = os.Stat(filepath.Join(dir, fileMeta))
	assert.NoError(t, err, "meta file must exist")
}

// TestCache_CorruptRecovery verifies invalid JSON in cache file triggers live fetch.
func TestCache_CorruptRecovery(t *testing.T) {
	dir := t.TempDir()
	data := singleProviderData()
	src := newStaticSource(data)

	// Write corrupt payload.
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePayload), []byte("{invalid json!!!"), 0o600))

	c := NewCache(src, WithCacheDir(dir), WithTTL(time.Hour))
	providers, err := c.Refresh(context.Background(), false)
	require.NoError(t, err)
	require.NotNil(t, providers)
	assert.Equal(t, 1, src.Calls(), "live fetch expected after corrupt cache")
}

// TestCache_StaleOnError verifies stale data served when source errors.
func TestCache_StaleOnError(t *testing.T) {
	dir := t.TempDir()
	data := singleProviderData()

	// Source that always errors.
	src := &staticSource{
		data: data,
		err:  fmt.Errorf("aim: fetch http://example.invalid: %w", &mockNetErr{temporary: false, timeout: false}),
	}

	// Pre-populate cache (expired so refresh will be attempted).
	writePayload(t, dir, data)
	writeMeta(t, dir, cacheMeta{
		LastFetch:  time.Now().UTC().Add(-2 * time.Hour),
		TTLSeconds: 3600,
	})

	c := NewCache(src, WithCacheDir(dir), WithTTL(time.Hour))
	providers, err := c.Refresh(context.Background(), false)
	require.NoError(t, err, "stale data should be returned without error")
	require.NotNil(t, providers)
	assert.NotNil(t, providers["acme"])
}

// mockNetErr is a minimal net.Error for testing isNetworkError in cache_test.
type mockNetErr struct {
	temporary bool
	timeout   bool
}

func (e *mockNetErr) Error() string   { return "network error" }
func (e *mockNetErr) Timeout() bool   { return e.timeout }
func (e *mockNetErr) Temporary() bool { return e.temporary }

// TestCache_ForceRefresh verifies force=true ignores TTL.
func TestCache_ForceRefresh_IgnoresTTL(t *testing.T) {
	dir := t.TempDir()
	data := singleProviderData()
	src := newStaticSource(data)

	// Pre-populate fresh cache.
	writePayload(t, dir, data)
	writeMeta(t, dir, cacheMeta{
		LastFetch:  time.Now().UTC(),
		TTLSeconds: 3600,
	})

	c := NewCache(src, WithCacheDir(dir), WithTTL(time.Hour))
	_, err := c.Refresh(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, 1, src.Calls(), "force refresh must call source even if fresh")
}

// TestCache_MaxSize_TreatedAsMiss verifies oversized cache file is treated as miss.
func TestCache_MaxSize_TreatedAsMiss(t *testing.T) {
	dir := t.TempDir()
	data := singleProviderData()
	src := newStaticSource(data)

	// Write valid JSON that's larger than MaxSize (10 bytes).
	b, err := json.Marshal(data)
	require.NoError(t, err)
	require.Greater(t, len(b), 10)
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePayload), b, 0o600))
	writeMeta(t, dir, cacheMeta{
		LastFetch:  time.Now().UTC(),
		TTLSeconds: 3600,
	})

	c := NewCache(src, WithCacheDir(dir), WithTTL(time.Hour), WithMaxSize(10))
	providers, err := c.Refresh(context.Background(), false)
	require.NoError(t, err)
	require.NotNil(t, providers)
	// Source was called because oversized file = miss.
	assert.Equal(t, 1, src.Calls())
}

// TestCache_Load_NoCache returns nil,nil when no cache exists.
func TestCache_Load_NoCache(t *testing.T) {
	dir := t.TempDir()
	src := newStaticSource(singleProviderData())
	c := NewCache(src, WithCacheDir(dir))
	providers, err := c.Load()
	require.NoError(t, err)
	assert.Nil(t, providers)
}

// TestCache_HTTPServer_Integration tests the full Refresh flow via httptest.
func TestCache_HTTPServer_Integration(t *testing.T) {
	data := singleProviderData()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		b, _ := json.Marshal(data)
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	src := &ModelsDevSource{URL: srv.URL}
	c := NewCache(src, WithCacheDir(t.TempDir()), WithTTL(time.Hour))

	providers, err := c.Refresh(context.Background(), true)
	require.NoError(t, err)
	assert.NotNil(t, providers)
	assert.Equal(t, 1, calls)
}

// TestCache_DiskRoundTrip_BackfillsProvider is the regression guard for the
// warm-cache provider-filter bug: Model.Provider is tagged `json:"-"`, so it
// never survives a serialise/deserialise cycle through the on-disk payload.
// The HTTP path backfills it from the parent map key; the disk-load path must
// do the same or every --provider filter silently matches nothing on the
// second and subsequent runs.
//
// This exercises the real disk path (write, then read back through a
// separately-constructed Cache) rather than an in-memory fake, because the
// in-memory fakes are precisely what let this ship.
func TestCache_DiskRoundTrip_BackfillsProvider(t *testing.T) {
	dir := t.TempDir()
	data := singleProviderData()
	src := newStaticSource(data)

	// First run: cold cache, fetch from source and persist to disk.
	c1 := NewCache(src, WithCacheDir(dir), WithTTL(time.Hour))
	first, err := c1.Refresh(context.Background(), true)
	require.NoError(t, err)
	require.NotNil(t, first["acme"])
	require.NotNil(t, first["acme"].Models["m1"])
	assert.Equal(t, "acme", first["acme"].Models["m1"].Provider,
		"cold fetch must populate Provider")

	// Confirm the on-disk payload really did drop the field — this is the
	// precondition that makes the backfill necessary rather than incidental.
	raw, err := os.ReadFile(filepath.Join(dir, filePayload))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"provider"`,
		"payload is expected to omit provider (json:\"-\"); "+
			"if this fails the tag changed and this test needs revisiting")

	// Second run: a brand-new Cache reading the warm on-disk cache.
	c2 := NewCache(newStaticSource(data), WithCacheDir(dir), WithTTL(time.Hour))
	warm, err := c2.Refresh(context.Background(), false)
	require.NoError(t, err)
	require.NotNil(t, warm["acme"])
	require.NotNil(t, warm["acme"].Models["m1"])
	assert.Equal(t, "acme", warm["acme"].Models["m1"].Provider,
		"warm disk load must backfill Provider from the parent map key")

	// Load() is the other disk-read entry point and must backfill too.
	loaded, err := c2.Load()
	require.NoError(t, err)
	require.NotNil(t, loaded["acme"])
	require.NotNil(t, loaded["acme"].Models["m1"])
	assert.Equal(t, "acme", loaded["acme"].Models["m1"].Provider,
		"Load must backfill Provider from the parent map key")
}

// TestRegistry_WarmCache_ProviderFilterMatches is the end-to-end guard: the
// user-visible symptom was `aim list --provider openai` returning 38 models on
// a cold cache and 0 on a warm one. Assert the filter returns identical
// results across both, driving a Registry through a real on-disk cache dir.
func TestRegistry_WarmCache_ProviderFilterMatches(t *testing.T) {
	dir := t.TempDir()
	data := singleProviderData()

	cold := NewRegistry(
		WithSource(newStaticSource(data)),
		WithCacheOpts(WithCacheDir(dir), WithTTL(time.Hour)),
	)
	coldModels, err := cold.Models(context.Background(), Filter{Provider: "acme"})
	require.NoError(t, err)
	require.Len(t, coldModels, 1, "cold cache must match the provider filter")
	assert.Equal(t, "acme", coldModels[0].Provider)

	// Fresh Registry over the now-warm cache dir: no source call should be
	// needed, and the provider filter must still match.
	warm := NewRegistry(
		WithSource(newStaticSource(data)),
		WithCacheOpts(WithCacheDir(dir), WithTTL(time.Hour)),
	)
	warmModels, err := warm.Models(context.Background(), Filter{Provider: "acme"})
	require.NoError(t, err)
	assert.Len(t, warmModels, 1,
		"warm cache must match the provider filter (was 0 before the backfill fix)")
	assert.Equal(t, coldModels, warmModels,
		"cold and warm results must be identical")
}
