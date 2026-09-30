package aim

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultCacheTTL is the default in-memory TTL for the registry's provider map.
	DefaultCacheTTL = 24 * time.Hour
)

// Registry is an AI model registry backed by one or more [Source] implementations.
// It provides lazy-loaded, concurrency-safe access to providers and models.
//
// By default it uses [ModelsDevSource] with a 24 h in-memory TTL. Supply
// [WithSource] to change the data source or [WithCacheOpts] to tune caching.
type Registry struct {
	source Source
	cache  *Cache // nil until buildCache is called

	// cacheOpts are applied to the Cache when it is built lazily.
	cacheOpts []CacheOption

	mu        sync.RWMutex
	providers map[string]*Provider // nil until first fetch
	fetchedAt time.Time
	ttl       time.Duration
}

// Option configures a [Registry].
type Option func(*Registry)

// WithSource overrides the data source used by the registry.
func WithSource(s Source) Option {
	return func(r *Registry) { r.source = s }
}

// WithCacheOpts passes [CacheOption] values to the underlying [Cache].
// Use [WithCacheDir] and [WithTTL] (defined in cache.go) as inputs.
//
//	reg := aim.NewRegistry(aim.WithCacheOpts(aim.WithCacheDir("/tmp/aim"), aim.WithTTL(time.Hour)))
func WithCacheOpts(opts ...CacheOption) Option {
	return func(r *Registry) { r.cacheOpts = append(r.cacheOpts, opts...) }
}

// NewRegistry creates a Registry with sensible defaults.
// Default source: [ModelsDevSource].
// Default cache: XDG cache at {xdg.CacheDir("hop")}/aim/ with 24 h TTL.
func NewRegistry(opts ...Option) *Registry {
	r := &Registry{
		source: &ModelsDevSource{},
		ttl:    DefaultCacheTTL,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Refresh forces a full cache refresh from the source.
func (r *Registry) Refresh(ctx context.Context) error {
	c := r.getCache()
	providers, err := c.Refresh(ctx, true)
	if err != nil {
		return fmt.Errorf("aim: refresh: %w", err)
	}
	r.mu.Lock()
	r.providers = providers
	r.fetchedAt = time.Now()
	r.mu.Unlock()
	return nil
}

// Providers returns all known providers sorted alphabetically by Provider.ID.
func (r *Registry) Providers(ctx context.Context) ([]Provider, error) {
	if err := r.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	r.mu.RLock()
	out := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, *p)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Models returns models matching all non-zero Filter fields (AND logic).
// Auto-fetches from the source on first call if the cache is empty.
//
// Modality filter uses subset containment:
// Filter.Input ⊆ Model.Modalities.Input (and same for Output).
func (r *Registry) Models(ctx context.Context, f Filter) ([]Model, error) {
	if err := r.ensureLoaded(ctx); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []Model
	for _, p := range r.providers {
		for _, m := range p.Models {
			if m == nil {
				continue
			}
			if matchesFilter(*m, f) {
				results = append(results, *m)
			}
		}
	}
	// stable order: provider then model ID
	sort.Slice(results, func(i, j int) bool {
		if results[i].Provider != results[j].Provider {
			return results[i].Provider < results[j].Provider
		}
		return results[i].ID < results[j].ID
	})
	return results, nil
}

// Get returns a model by provider ID and model ID (wire IDs, not display names).
// Returns (model, true, nil) when found; (zero, false, nil) when not found.
func (r *Registry) Get(ctx context.Context, providerID, modelID string) (Model, bool, error) {
	if err := r.ensureLoaded(ctx); err != nil {
		return Model{}, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[providerID]
	if !ok {
		return Model{}, false, nil
	}
	m, ok := p.Models[modelID]
	if !ok || m == nil {
		return Model{}, false, nil
	}
	return *m, true, nil
}

// Query parses q with [ParseQuery] then calls [Registry.Models].
func (r *Registry) Query(ctx context.Context, q string) ([]Model, error) {
	f, err := ParseQuery(q)
	if err != nil {
		return nil, err
	}
	return r.Models(ctx, f)
}

// ensureLoaded fetches from the source if the cache is empty or TTL expired.
func (r *Registry) ensureLoaded(ctx context.Context) error {
	r.mu.RLock()
	loaded := r.providers != nil && (r.ttl <= 0 || time.Since(r.fetchedAt) < r.ttl)
	r.mu.RUnlock()
	if loaded {
		return nil
	}

	c := r.getCache()
	providers, err := c.Refresh(ctx, false)
	if err != nil {
		return fmt.Errorf("aim: load: %w", err)
	}
	r.mu.Lock()
	r.providers = providers
	r.fetchedAt = time.Now()
	r.mu.Unlock()
	return nil
}

// getCache returns the Registry's Cache, building it lazily on first use.
func (r *Registry) getCache() *Cache {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache == nil {
		r.cache = NewCache(r.source, r.cacheOpts...)
	}
	return r.cache
}

// Cache returns the Registry's underlying [Cache], building it lazily
// on first access. Callers use it to read provenance ([Cache.Meta]) for
// envelope output without forcing a refresh.
func (r *Registry) Cache() *Cache {
	return r.getCache()
}

// SourceURL returns the upstream source URL the registry's [Source] will
// fetch from. Returns [DefaultSourceURL] when the active source is the
// default [ModelsDevSource] with no URL override, or the empty string
// when the source is a custom implementation that doesn't expose a URL.
func (r *Registry) SourceURL() string {
	r.mu.RLock()
	src := r.source
	r.mu.RUnlock()
	if mds, ok := src.(*ModelsDevSource); ok {
		if mds.URL != "" {
			return mds.URL
		}
		return DefaultSourceURL
	}
	return ""
}

// matchesFilter reports whether m satisfies every non-zero field in f.
func matchesFilter(m Model, f Filter) bool {
	if f.Provider != "" && m.Provider != f.Provider {
		return false
	}
	if f.Family != "" && m.Family != f.Family {
		return false
	}
	if !subsetOf(f.Input, m.Modalities.Input) {
		return false
	}
	if !subsetOf(f.Output, m.Modalities.Output) {
		return false
	}
	if f.ToolCall != nil && m.ToolCall != *f.ToolCall {
		return false
	}
	if f.Reasoning != nil && m.Reasoning != *f.Reasoning {
		return false
	}
	if f.OpenWeights != nil && m.OpenWeights != *f.OpenWeights {
		return false
	}
	if f.StructuredOutput != nil && m.StructuredOutput != *f.StructuredOutput {
		return false
	}
	if f.Temperature != nil && m.Temperature != *f.Temperature {
		return false
	}
	if f.Query != "" && !matchesQuery(m, f.Query) {
		return false
	}
	return true
}

// subsetOf reports whether every element of need is present in have.
func subsetOf(need, have []string) bool {
	if len(need) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(have))
	for _, v := range have {
		set[v] = struct{}{}
	}
	for _, v := range need {
		if _, ok := set[v]; !ok {
			return false
		}
	}
	return true
}

// matchesQuery performs a case-insensitive substring match against Model.ID
// and Model.Name.
func matchesQuery(m Model, q string) bool {
	lower := strings.ToLower(q)
	return strings.Contains(strings.ToLower(m.ID), lower) ||
		strings.Contains(strings.ToLower(m.Name), lower)
}
