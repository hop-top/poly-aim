package aim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"hop.top/kit/go/core/breaker"
)

const (
	// DefaultSourceURL is the default models.dev API endpoint.
	DefaultSourceURL = "https://models.dev/api.json"
	// DefaultMaxResponseSize is the default cap on response body size (50 MB).
	DefaultMaxResponseSize = 50 * 1024 * 1024

	// SourceBreakerName is the registry key for the upstream-fetch
	// breaker. Exposed so status providers and tests can [breaker.Lookup]
	// the live fuse without re-registering it.
	SourceBreakerName = "aim-source"

	// sourceBreakerFailureThreshold is the consecutive failure count that
	// trips the breaker (Closed → Open).
	sourceBreakerFailureThreshold = 3
	// sourceBreakerSuccessThreshold is the consecutive success count in
	// HalfOpen that closes the breaker again.
	sourceBreakerSuccessThreshold = 1
	// sourceBreakerResetDelay is the wait in Open before HalfOpen probes.
	sourceBreakerResetDelay = 30 * time.Second
)

// sourceBreakerOnce guards lazy registration so importing the package
// twice (e.g. in tests) does not panic on duplicate registration. The
// fuse lives in [breaker]'s process-wide registry and is reachable via
// [breaker.Lookup]([SourceBreakerName]).
var (
	sourceBreakerOnce sync.Once
	sourceBreaker     breaker.Breaker
)

// SourceBreaker returns the process-wide circuit breaker guarding
// [ModelsDevSource.Fetch]. First call registers the fuse with kit's
// breaker registry under [SourceBreakerName]; subsequent calls return
// the same instance. Safe for concurrent use.
func SourceBreaker() breaker.Breaker {
	sourceBreakerOnce.Do(func() {
		sourceBreaker = breaker.New(SourceBreakerName,
			breaker.WithCircuit(breaker.CircuitOpts{
				FailureThreshold: sourceBreakerFailureThreshold,
				SuccessThreshold: sourceBreakerSuccessThreshold,
				Delay:            sourceBreakerResetDelay,
			}),
		)
	})
	return sourceBreaker
}

// ModelsDevSource fetches provider data from models.dev/api.json.
// It implements [Source].
type ModelsDevSource struct {
	// URL overrides the fetch endpoint. Defaults to [DefaultSourceURL].
	URL string
	// MaxResponseSize caps the response body. Defaults to [DefaultMaxResponseSize].
	MaxResponseSize int64
	// Client is the HTTP client used for requests. Defaults to a 30s-timeout client.
	Client *http.Client
	// Breaker overrides the circuit breaker guarding Fetch. Defaults to
	// the process-wide [SourceBreaker]. Tests inject a private breaker
	// to avoid leaking state across cases.
	Breaker breaker.Breaker
}

// Fetch retrieves and deserialises the provider map from models.dev.
// Unknown JSON fields are silently ignored (forward-compatible).
// Returns an error if the response body exceeds MaxResponseSize.
//
// The HTTP round trip is guarded by [SourceBreaker]: three consecutive
// transport failures trip the fuse, after which Fetch fails fast with
// [breaker.ErrBrokenCircuit] for the configured reset delay before a
// HalfOpen probe is allowed. Successful responses immediately close the
// fuse. The breaker treats non-2xx HTTP status as a failure so upstream
// 5xx outages count toward the threshold.
func (s *ModelsDevSource) Fetch(ctx context.Context) (map[string]*Provider, error) {
	url := s.URL
	if url == "" {
		url = DefaultSourceURL
	}
	b := s.Breaker
	if b == nil {
		b = SourceBreaker()
	}

	// Run the fetch through the breaker's executor so failsafe-go's
	// count-based ratio (FailureThreshold/N) sees the real outcome —
	// success closes the fuse, transport / 5xx failure ticks the
	// failure counter and trips Open at the threshold. The executor
	// short-circuits with its own broken-circuit error when already
	// Open; we map that onto kit's [breaker.ErrBrokenCircuit] sentinel
	// so callers see a uniform fast-fail regardless of which layer
	// detected the open state.
	got, runErr := b.Executor().Get(func() (any, error) {
		return s.doFetch(ctx, url)
	})
	if runErr != nil {
		if b.State() == breaker.Open && !isLikelyTransportErr(runErr) {
			return nil, fmt.Errorf("aim: fetch %s: %w", url, breaker.ErrBrokenCircuit)
		}
		return nil, runErr
	}
	if got == nil {
		return nil, nil
	}
	return got.(map[string]*Provider), nil
}

// isLikelyTransportErr reports whether err mentions our own fetch-error
// prefixes ("aim: fetch", "aim: read", "aim: decode"). The breaker
// executor returns its sentinel "circuit breaker open" with no aim
// prefix, so we use the presence of the aim prefix as a proxy for "the
// fetch fn actually ran and produced this error" — sidestepping a
// direct dep on failsafe-go's ErrOpen sentinel while still preserving
// the breaker-open vs transport-failure distinction.
func isLikelyTransportErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, prefix := range []string{
		"aim: fetch",
		"aim: read",
		"aim: decode",
		"aim: provider map key",
		"aim: create request",
	} {
		if len(msg) >= len(prefix) && msg[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// doFetch is the unguarded HTTP path. Extracted so [Fetch] can record
// success/failure outcomes on the breaker around a single call site.
func (s *ModelsDevSource) doFetch(ctx context.Context, url string) (map[string]*Provider, error) {
	maxSize := s.MaxResponseSize
	if maxSize == 0 {
		maxSize = DefaultMaxResponseSize
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("aim: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aim: fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aim: fetch %s: unexpected status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("aim: read response: %w", err)
	}
	if int64(len(body)) > maxSize {
		return nil, fmt.Errorf("aim: response from %s exceeds max size (%d bytes)", url, maxSize)
	}

	var raw map[string]*Provider
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("aim: decode response: %w", err)
	}

	// Validate map key == Provider.ID and backfill Model.Provider.
	for key, p := range raw {
		if p == nil {
			delete(raw, key)
			continue
		}
		if p.ID == "" {
			p.ID = key
		}
		if p.ID != key {
			return nil, fmt.Errorf("aim: provider map key %q != provider id %q", key, p.ID)
		}
	}
	backfillProviders(raw)

	return raw, nil
}

// backfillProviders populates the derived [Model.Provider] field from the
// parent map key for every model in providers, dropping nil entries.
//
// Model.Provider is tagged `json:"-"` — it is not part of the models.dev wire
// format and does not survive any JSON round trip. Every path that produces a
// provider map from JSON (the HTTP fetch in [ModelsDevSource.doFetch] and the
// on-disk cache load in [Cache.loadFromDisk]) must call this, or downstream
// provider filtering and sorting silently degrade: a zero Provider makes
// Filter.Provider match nothing at all.
//
// Safe to call repeatedly; it is an idempotent assignment.
func backfillProviders(providers map[string]*Provider) {
	for key, p := range providers {
		if p == nil {
			delete(providers, key)
			continue
		}
		id := p.ID
		if id == "" {
			id = key
		}
		for _, m := range p.Models {
			if m != nil {
				m.Provider = id
			}
		}
	}
}

// IsBreakerOpen reports whether err originated from a tripped [SourceBreaker].
// Provided so callers (status providers, refresh paths) can distinguish a
// fast-fail from a live transport failure without unwrapping by hand.
func IsBreakerOpen(err error) bool {
	return errors.Is(err, breaker.ErrBrokenCircuit)
}
