package aim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"hop.top/kit/go/core/xdg"
)

const (
	defaultTTL     = 24 * time.Hour
	defaultMaxSize = 50 * 1024 * 1024 // 50 MB

	filePayload = "models-dev.json"
	fileMeta    = "meta.json"
	fileLock    = ".lock"

	lockRetryInterval = 100 * time.Millisecond
	lockTimeout       = 5 * time.Second
)

// cacheMeta is persisted to meta.json alongside the payload.
type cacheMeta struct {
	LastFetch  time.Time `json:"last_fetch"`
	ETag       string    `json:"etag,omitempty"`
	TTLSeconds int64     `json:"ttl_seconds"`
}

// Meta is a public, copied view of the on-disk cache metadata. Callers
// use it to attach provenance ([output.Metadata]) to rendered output
// without reaching into unexported types or re-reading meta.json.
//
// Present reports whether meta.json exists on disk. When false the
// other fields are zero. Callers should treat a non-Present meta as
// "first run, never fetched".
type Meta struct {
	Present   bool
	LastFetch time.Time
	ETag      string
	TTL       time.Duration
}

// CacheOption configures a [Cache].
type CacheOption func(*Cache)

// WithCacheDir overrides the cache directory.
func WithCacheDir(dir string) CacheOption {
	return func(c *Cache) { c.Dir = dir }
}

// WithTTL overrides the cache TTL.
func WithTTL(d time.Duration) CacheOption {
	return func(c *Cache) { c.TTL = d }
}

// WithMaxSize overrides the maximum cache file size in bytes.
func WithMaxSize(n int64) CacheOption {
	return func(c *Cache) { c.MaxSize = n }
}

// Cache is an XDG-backed file cache for provider data.
// Files are stored in Dir: models-dev.json (payload) and meta.json (metadata).
type Cache struct {
	// Dir is the cache directory. Defaults to xdg.CacheDir("hop")+"/aim".
	Dir string
	// TTL is the cache lifetime. Defaults to 24h.
	TTL time.Duration
	// MaxSize is the maximum accepted payload file size. Defaults to 50 MB.
	MaxSize int64
	// Source is called on cache miss or expiry. Required.
	Source Source
}

// NewCache creates a Cache backed by src with optional configuration.
func NewCache(src Source, opts ...CacheOption) *Cache {
	c := &Cache{Source: src}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Cache) dir() (string, error) {
	if c.Dir != "" {
		return c.Dir, nil
	}
	base, err := xdg.CacheDir("hop")
	if err != nil {
		return "", fmt.Errorf("aim: cache dir: %w", err)
	}
	return filepath.Join(base, "aim"), nil
}

func (c *Cache) ttl() time.Duration {
	if c.TTL > 0 {
		return c.TTL
	}
	return defaultTTL
}

func (c *Cache) maxSize() int64 {
	if c.MaxSize > 0 {
		return c.MaxSize
	}
	return defaultMaxSize
}

// Load returns cached data without refreshing. Returns nil,nil if no cache exists.
func (c *Cache) Load() (map[string]*Provider, error) {
	dir, err := c.dir()
	if err != nil {
		return nil, err
	}
	data, _, err := c.loadFromDisk(dir)
	return data, err
}

// Refresh fetches fresh data if TTL expired (or force=true).
// Returns cached data if still fresh.
// On network error: returns stale data if available, else error.
func (c *Cache) Refresh(ctx context.Context, force bool) (map[string]*Provider, error) {
	dir, err := c.dir()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("aim: create cache dir: %w", err)
	}

	// Load existing cache (may be nil).
	cached, meta, loadErr := c.loadFromDisk(dir)
	// loadErr is non-nil only for non-corruption I/O errors; corruption is
	// handled inside loadFromDisk by deleting bad files and returning nil.

	// Check freshness unless force=true.
	if !force && cached != nil && loadErr == nil {
		ttl := c.ttl()
		if meta != nil && meta.TTLSeconds > 0 {
			ttl = time.Duration(meta.TTLSeconds) * time.Second
		}
		if time.Since(meta.LastFetch) < ttl {
			return cached, nil
		}
	}

	// Acquire lockfile.
	unlock, err := c.acquireLock(dir)
	if err != nil {
		// If lock acquisition fails, serve stale if available.
		if cached != nil {
			return cached, nil
		}
		return nil, fmt.Errorf("aim: acquire lock: %w", err)
	}
	defer unlock()

	// Re-check freshness after acquiring lock (another process may have refreshed).
	if !force {
		cached2, meta2, _ := c.loadFromDisk(dir)
		if cached2 != nil && meta2 != nil {
			ttl := c.ttl()
			if meta2.TTLSeconds > 0 {
				ttl = time.Duration(meta2.TTLSeconds) * time.Second
			}
			if time.Since(meta2.LastFetch) < ttl {
				return cached2, nil
			}
			// Update local cached/meta for ETag use below.
			cached = cached2
			meta = meta2
		}
	}

	// Perform the fetch.
	etag := ""
	if meta != nil {
		etag = meta.ETag
	}

	providers, newETag, err := c.fetchWithETag(ctx, etag)
	if err != nil {
		if isNetworkError(err) && cached != nil {
			return cached, nil
		}
		return nil, err
	}

	// 304 Not Modified — payload unchanged; bump last_fetch.
	if providers == nil {
		providers = cached
	}

	newMeta := &cacheMeta{
		LastFetch:  time.Now().UTC(),
		ETag:       newETag,
		TTLSeconds: int64(c.ttl().Seconds()),
	}

	if err := c.writeToDisk(dir, providers, newMeta); err != nil {
		// Disk-full or other write error: return live data but propagate error.
		return providers, fmt.Errorf("aim: write cache: %w", err)
	}

	return providers, nil
}

// fetchWithETag calls Source.Fetch. When etag is non-empty it sets
// If-None-Match and interprets a 304 as (nil, etag, nil).
// For non-ETag-aware sources it just calls Fetch directly.
func (c *Cache) fetchWithETag(ctx context.Context, etag string) (map[string]*Provider, string, error) {
	type etagSource interface {
		FetchWithETag(ctx context.Context, etag string) (map[string]*Provider, string, error)
	}
	if es, ok := c.Source.(etagSource); ok {
		return es.FetchWithETag(ctx, etag)
	}
	// Fall back: plain fetch, no ETag.
	providers, err := c.Source.Fetch(ctx)
	if err != nil {
		return nil, "", err
	}
	return providers, "", nil
}

// loadFromDisk reads payload + meta from dir.
// On JSON corruption: deletes corrupt files, returns nil,nil,nil.
// On absence: returns nil,nil,nil.
func (c *Cache) loadFromDisk(dir string) (map[string]*Provider, *cacheMeta, error) {
	payloadPath := filepath.Join(dir, filePayload)
	metaPath := filepath.Join(dir, fileMeta)

	// Read payload.
	payloadBytes, err := os.ReadFile(payloadPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("aim: read cache payload: %w", err)
	}

	// Enforce max size.
	if int64(len(payloadBytes)) > c.maxSize() {
		_ = os.Remove(payloadPath)
		_ = os.Remove(metaPath)
		return nil, nil, nil
	}

	// Decode payload.
	var providers map[string]*Provider
	if err := json.Unmarshal(payloadBytes, &providers); err != nil {
		_ = os.Remove(payloadPath)
		_ = os.Remove(metaPath)
		return nil, nil, nil // corruption — fall through to live fetch
	}

	// Model.Provider is `json:"-"` and so is absent from the payload we
	// just decoded. Re-derive it from the parent map key, mirroring the
	// HTTP fetch path — without this every provider filter matches
	// nothing once the cache is warm.
	backfillProviders(providers)

	// Read meta (optional — tolerate absence).
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return providers, nil, nil
		}
		return providers, nil, nil
	}
	var meta cacheMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		_ = os.Remove(metaPath)
		return providers, nil, nil
	}

	return providers, &meta, nil
}

// writeToDisk atomically writes payload + meta to dir.
func (c *Cache) writeToDisk(dir string, providers map[string]*Provider, meta *cacheMeta) error {
	if err := atomicWriteJSON(filepath.Join(dir, filePayload), providers); err != nil {
		return err
	}
	return atomicWriteJSON(filepath.Join(dir, fileMeta), meta)
}

// atomicWriteJSON marshals v to a temp file in the same dir, then renames it.
func atomicWriteJSON(dst string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("aim: marshal %s: %w", filepath.Base(dst), err)
	}

	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return fmt.Errorf("aim: create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("aim: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("aim: close temp file: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("aim: rename %s: %w", filepath.Base(dst), err)
	}
	return nil
}

// acquireLock creates a lockfile using O_CREATE|O_EXCL (cross-platform).
// Retries with backoff up to lockTimeout. Returns an unlock func.
func (c *Cache) acquireLock(dir string) (func(), error) {
	lockPath := filepath.Join(dir, fileLock)
	deadline := time.Now().Add(lockTimeout)

	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("aim: lockfile: %w", err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("aim: lock timeout after %s", lockTimeout)
		}
		time.Sleep(lockRetryInterval)
	}
}

// Meta returns a copy of the on-disk cache metadata. When meta.json is
// absent (first run, corrupt, or never written), Present is false and
// the remaining fields stay zero. Network and disk errors that prevent
// reading the file surface as Present=false; callers fall back to a
// "never fetched" provenance envelope without panicking.
func (c *Cache) Meta() Meta {
	dir, err := c.dir()
	if err != nil {
		return Meta{}
	}
	metaBytes, err := os.ReadFile(filepath.Join(dir, fileMeta))
	if err != nil {
		return Meta{}
	}
	var m cacheMeta
	if err := json.Unmarshal(metaBytes, &m); err != nil {
		return Meta{}
	}
	out := Meta{
		Present:   true,
		LastFetch: m.LastFetch,
		ETag:      m.ETag,
	}
	if m.TTLSeconds > 0 {
		out.TTL = time.Duration(m.TTLSeconds) * time.Second
	}
	return out
}

// isNetworkError reports whether err is a transient network/DNS error.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}
