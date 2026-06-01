package aim

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ptr returns a pointer to v; used for tristate bool Filter fields.
func ptr[T any](v T) *T { return &v }

// fixtureServer starts an httptest.Server serving testdata/api-fixture.json.
// Caller must call srv.Close().
func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile("testdata/api-fixture.json")
	require.NoError(t, err, "read testdata/api-fixture.json")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
}

// fixtureRegistry returns a Registry backed by the fixture httptest server.
// TTL is set to 0 so cache never expires mid-test.
func fixtureRegistry(t *testing.T, srv *httptest.Server) *Registry {
	t.Helper()
	dir := t.TempDir()
	return NewRegistry(
		WithSource(&ModelsDevSource{URL: srv.URL}),
		WithCacheOpts(WithCacheDir(dir), WithTTL(24*time.Hour)),
	)
}

// modelIDs extracts sorted IDs from a model slice for easy assertion.
func modelIDs(models []Model) []string {
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	sort.Strings(ids)
	return ids
}

// TestUS01_ImageInputModels — find models with image input.
func TestUS01_ImageInputModels(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	reg := fixtureRegistry(t, srv)
	models, err := reg.Models(context.Background(), Filter{Input: []string{"image"}})
	require.NoError(t, err)

	ids := modelIDs(models)
	assert.Equal(t, []string{"claude-opus-4-5", "claude-sonnet-4-5", "gpt-4o"}, ids)
	assert.Len(t, models, 3)
}

// TestUS02_ImageOutputModels — find models that generate images (none in fixture).
func TestUS02_ImageOutputModels(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	reg := fixtureRegistry(t, srv)
	models, err := reg.Models(context.Background(), Filter{Output: []string{"image"}})
	require.NoError(t, err)
	assert.Empty(t, models)
}

// TestUS03_ToolCallModels — compare providers for a capability.
func TestUS03_ToolCallModels(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	reg := fixtureRegistry(t, srv)
	models, err := reg.Models(context.Background(), Filter{ToolCall: ptr(true)})
	require.NoError(t, err)

	// gpt-4o, o3, claude-opus-4-5, claude-sonnet-4-5
	assert.Len(t, models, 4)
	ids := modelIDs(models)
	assert.Contains(t, ids, "gpt-4o")
	assert.Contains(t, ids, "o3")
	assert.Contains(t, ids, "claude-opus-4-5")
	assert.Contains(t, ids, "claude-sonnet-4-5")
	assert.NotContains(t, ids, "llama3.2")
}

// TestUS04_ModelDetailLookup — model detail lookup by provider+model ID.
func TestUS04_ModelDetailLookup(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	reg := fixtureRegistry(t, srv)
	m, ok, err := reg.Get(context.Background(), "openai", "gpt-4o")
	require.NoError(t, err)
	require.True(t, ok)

	assert.Equal(t, "gpt-4o", m.ID)
	assert.Equal(t, "openai", m.Provider)
	assert.Equal(t, "GPT-4o", m.Name)
	assert.True(t, m.ToolCall)
	assert.False(t, m.Reasoning)
	assert.Contains(t, m.Modalities.Input, "image")
	assert.Contains(t, m.Modalities.Output, "text")
}

// TestUS05_OfflineUsageFromCache — refresh once, then Load() without network.
func TestUS05_OfflineUsageFromCache(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	dir := t.TempDir()
	src := &ModelsDevSource{URL: srv.URL}
	cache := NewCache(src, WithCacheDir(dir), WithTTL(24*time.Hour))

	// Refresh once to populate cache.
	providers, err := cache.Refresh(context.Background(), true)
	require.NoError(t, err)
	require.NotNil(t, providers)
	initialCount := len(providers)

	// Now Load() directly — no network call needed.
	loaded, err := cache.Load()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Len(t, loaded, initialCount)

	// Verify we have the expected providers.
	assert.Contains(t, loaded, "openai")
	assert.Contains(t, loaded, "anthropic")
	assert.Contains(t, loaded, "ollama")
}

// TestUS06_ProgrammaticProviderFilter — Registry.Models with Provider filter.
func TestUS06_ProgrammaticProviderFilter(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	reg := fixtureRegistry(t, srv)
	models, err := reg.Models(context.Background(), Filter{Provider: "anthropic"})
	require.NoError(t, err)

	assert.Len(t, models, 2)
	for _, m := range models {
		assert.Equal(t, "anthropic", m.Provider)
	}
	ids := modelIDs(models)
	assert.Contains(t, ids, "claude-opus-4-5")
	assert.Contains(t, ids, "claude-sonnet-4-5")
}

// TestUS07_StringQueryMatchesFilter — Query("provider:openai") == Models(Filter{Provider:"openai"}).
func TestUS07_StringQueryMatchesFilter(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	reg := fixtureRegistry(t, srv)
	ctx := context.Background()

	byQuery, err := reg.Query(ctx, "provider:openai")
	require.NoError(t, err)

	byFilter, err := reg.Models(ctx, Filter{Provider: "openai"})
	require.NoError(t, err)

	assert.Equal(t, modelIDs(byFilter), modelIDs(byQuery))
	assert.Len(t, byQuery, 2)
}

// customSource is a test Source returning a single in-memory provider.
type customSource struct {
	providers map[string]*Provider
}

func (s *customSource) Fetch(_ context.Context) (map[string]*Provider, error) {
	return s.providers, nil
}

// TestUS08_CustomSourceInternalModels — custom Source; Registry picks it up.
func TestUS08_CustomSourceInternalModels(t *testing.T) {
	custom := &customSource{
		providers: map[string]*Provider{
			"internal": {
				ID:   "internal",
				Name: "Internal",
				Models: map[string]*Model{
					"model-x": {
						ID:       "model-x",
						Name:     "Model X",
						Provider: "internal",
						Modalities: Modalities{
							Input:  []string{"text"},
							Output: []string{"text"},
						},
					},
				},
			},
		},
	}

	dir := t.TempDir()
	reg := NewRegistry(
		WithSource(custom),
		WithCacheOpts(WithCacheDir(dir), WithTTL(24*time.Hour)),
	)

	models, err := reg.Models(context.Background(), Filter{Provider: "internal"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "model-x", models[0].ID)
	assert.Equal(t, "internal", models[0].Provider)
}

// errSource always returns a simulated network error from Fetch.
type errSource struct{ err error }

func (s *errSource) Fetch(_ context.Context) (map[string]*Provider, error) {
	return nil, s.err
}

// e2eNetErr is a minimal net.Error so isNetworkError returns true.
type e2eNetErr struct{}

func (e *e2eNetErr) Error() string   { return "simulated network error" }
func (e *e2eNetErr) Timeout() bool   { return false }
func (e *e2eNetErr) Temporary() bool { return true }


// TestUS09_StaleCacheOnRefreshFailure — stale cache served when refresh fails.
func TestUS09_StaleCacheOnRefreshFailure(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	dir := t.TempDir()

	// Phase 1: populate cache with fixture data.
	goodSrc := &ModelsDevSource{URL: srv.URL}
	cache := NewCache(goodSrc, WithCacheDir(dir), WithTTL(24*time.Hour))
	providers, err := cache.Refresh(context.Background(), true)
	require.NoError(t, err)
	require.NotNil(t, providers)

	// Phase 2: replace source with one that returns a network error.
	failSrc := &errSource{err: &e2eNetErr{}}
	stalecache := NewCache(failSrc, WithCacheDir(dir), WithTTL(1*time.Nanosecond))

	// Force TTL expiry.
	time.Sleep(2 * time.Millisecond)

	stale, err := stalecache.Refresh(context.Background(), true)
	// Should return stale data, not an error.
	require.NoError(t, err, "expected stale data served, not error")
	require.NotNil(t, stale)
	assert.Contains(t, stale, "openai")
	assert.Contains(t, stale, "anthropic")
}

// TestUS10_UnknownFieldsSilentlyIgnored — extra JSON fields don't break unmarshal.
func TestUS10_UnknownFieldsSilentlyIgnored(t *testing.T) {
	// Load fixture and append an extra field to verify forward-compat.
	fixtureWithExtra := `{
  "openai": {
    "id": "openai",
    "name": "OpenAI",
    "_extra_field": "value",
    "models": {
      "gpt-4o": {
        "id": "gpt-4o",
        "name": "GPT-4o",
        "_extra_model_field": 42,
        "modalities": {"input": ["text", "image"], "output": ["text"]},
        "tool_call": true,
        "limit": {"context": 128000}
      }
    }
  }
}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixtureWithExtra))
	}))
	defer srv.Close()

	src := &ModelsDevSource{URL: srv.URL}
	providers, err := src.Fetch(context.Background())
	require.NoError(t, err)
	require.NotNil(t, providers["openai"])
	assert.Equal(t, "gpt-4o", providers["openai"].Models["gpt-4o"].ID)
}

// TestUS11_EmptyResultClearSignal — empty result is ([], nil), not an error.
func TestUS11_EmptyResultClearSignal(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	reg := fixtureRegistry(t, srv)
	models, err := reg.Models(context.Background(), Filter{Input: []string{"video"}})
	require.NoError(t, err)
	// nil slice and empty slice both signal "no results"; no error is the key invariant.
	assert.Empty(t, models)
}

// TestUS12_ForceRefreshIgnoresTTL — Refresh(force=true) bypasses TTL.
func TestUS12_ForceRefreshIgnoresTTL(t *testing.T) {
	// Start with the standard fixture.
	srv := fixtureServer(t)
	defer srv.Close()

	dir := t.TempDir()
	src := &ModelsDevSource{URL: srv.URL}
	cache := NewCache(src, WithCacheDir(dir), WithTTL(24*time.Hour))

	// Initial non-forced refresh — populates cache.
	providers1, err := cache.Refresh(context.Background(), false)
	require.NoError(t, err)
	assert.Len(t, providers1, 3, "initial: 3 providers")

	// Switch source to a different fixture (2 providers only).
	slim := `{
  "openai": {
    "id": "openai",
    "name": "OpenAI",
    "models": {
      "gpt-4o": {
        "id": "gpt-4o",
        "name": "GPT-4o",
        "modalities": {"input": ["text", "image"], "output": ["text"]},
        "tool_call": true,
        "limit": {}
      }
    }
  },
  "anthropic": {
    "id": "anthropic",
    "name": "Anthropic",
    "models": {
      "claude-opus-4-5": {
        "id": "claude-opus-4-5",
        "name": "Claude Opus 4.5",
        "modalities": {"input": ["text", "image"], "output": ["text"]},
        "tool_call": true,
        "limit": {}
      }
    }
  }
}`
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(slim))
	}))
	defer srv2.Close()

	// Replace source on same cache dir — force=false respects TTL (returns old data).
	cacheOld := NewCache(&ModelsDevSource{URL: srv2.URL}, WithCacheDir(dir), WithTTL(24*time.Hour))
	providers2, err := cacheOld.Refresh(context.Background(), false)
	require.NoError(t, err)
	assert.Len(t, providers2, 3, "non-force with fresh TTL: still 3 providers")

	// Force refresh — bypasses TTL, picks up new source.
	providers3, err := cacheOld.Refresh(context.Background(), true)
	require.NoError(t, err)
	assert.Len(t, providers3, 2, "force refresh: 2 providers from new source")
}

// TestUS13_ProviderFilterUnambiguous — Provider:"openai" never returns anthropic models.
func TestUS13_ProviderFilterUnambiguous(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	reg := fixtureRegistry(t, srv)
	models, err := reg.Models(context.Background(), Filter{Provider: "openai"})
	require.NoError(t, err)
	require.NotEmpty(t, models)

	for _, m := range models {
		assert.Equal(t, "openai", m.Provider,
			"model %q must have provider=openai, got %q", m.ID, m.Provider)
		assert.NotEqual(t, "anthropic", m.Provider)
	}
}
