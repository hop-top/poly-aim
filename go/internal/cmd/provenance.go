package cmd

import (
	"time"

	"hop.top/aim"
	"hop.top/kit/go/console/output"
)

// provenanceFromCache builds an [output.Metadata] envelope describing the
// data the caller is about to render. It reads the cache's persisted
// metadata via [aim.Cache.Meta] and pairs it with the registry's source
// URL.
//
// When meta.json is present, the envelope reports the recorded
// fetched_at, sets cached=true (the call hit the cache layer), and
// computes cache_age as time.Since(LastFetch).
//
// When meta.json is absent (first run, corrupt, or pre-fetch view),
// the envelope falls back to a "live" shape: fetched_at = now,
// cached=false, no cache_age. This is the right shape for the
// post-refresh path in [aim.Cache.Refresh] callers as well.
//
// sourceURL may be empty when a custom [aim.Source] is wired; the
// envelope is still emitted so the wire shape stays stable.
func provenanceFromCache(c *aim.Cache, sourceURL string) output.Metadata {
	m := output.Metadata{
		Source: sourceURL,
		Method: "http_get_cached",
	}
	if c == nil {
		m.Method = "http_get"
		m.FetchedAt = time.Now().UTC()
		return m
	}
	meta := c.Meta()
	if !meta.Present || meta.LastFetch.IsZero() {
		// No cache on disk — treat as a live response. Callers that
		// just performed an explicit refresh land here too.
		m.Method = "http_get"
		m.FetchedAt = time.Now().UTC()
		return m
	}
	m.FetchedAt = meta.LastFetch
	m.Cached = true
	m.CacheAge = time.Since(meta.LastFetch)
	return m
}

// provenanceForRefresh returns the envelope to attach to a successful
// [aim.Registry.Refresh] response. Method is "http_get", Cached is
// false, FetchedAt is now (the just-completed fetch). The cache is
// still consulted only to anchor the source URL; callers pass it in.
func provenanceForRefresh(sourceURL string) output.Metadata {
	return output.Metadata{
		Source:    sourceURL,
		FetchedAt: time.Now().UTC(),
		Method:    "http_get",
		Cached:    false,
	}
}
