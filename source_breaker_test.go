package aim

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hop.top/kit/go/core/breaker"
)

// newPrivateSourceBreaker returns a fresh breaker pinned to the test
// failure-count thresholds + a tiny reset delay. Registering under a
// unique name + t.Cleanup keeps the process-wide registry clean.
func newPrivateSourceBreaker(t *testing.T, resetDelay time.Duration) breaker.Breaker {
	t.Helper()
	name := "aim-source-test/" + t.Name()
	t.Cleanup(func() { breaker.Unregister(name) })
	return breaker.New(name,
		breaker.WithCircuit(breaker.CircuitOpts{
			FailureThreshold: sourceBreakerFailureThreshold,
			SuccessThreshold: sourceBreakerSuccessThreshold,
			Delay:            resetDelay,
		}),
	)
}

func TestSourceBreaker_ClosedOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(minimalFixture))
	}))
	defer srv.Close()

	b := newPrivateSourceBreaker(t, 100*time.Millisecond)
	src := &ModelsDevSource{URL: srv.URL, Breaker: b}

	for i := 0; i < 5; i++ {
		_, err := src.Fetch(context.Background())
		require.NoErrorf(t, err, "iter %d", i)
	}
	assert.Equal(t, breaker.Closed, b.State(),
		"breaker must stay closed across many successful fetches")
}

func TestSourceBreaker_OpensAfterConsecutiveFailures(t *testing.T) {
	// Always 500 — every fetch is a failure outcome.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	b := newPrivateSourceBreaker(t, time.Hour) // long delay — verify Open stays Open
	src := &ModelsDevSource{URL: srv.URL, Breaker: b}

	// Three consecutive failures trip the breaker (FailureThreshold=3).
	for i := 0; i < sourceBreakerFailureThreshold; i++ {
		_, err := src.Fetch(context.Background())
		require.Errorf(t, err, "iter %d should hit upstream and fail", i)
		assert.NotErrorIs(t, err, breaker.ErrBrokenCircuit,
			"iter %d should be a transport-class failure, not a fast-fail", i)
	}

	assert.Equal(t, breaker.Open, b.State(),
		"breaker must trip Open after %d consecutive failures",
		sourceBreakerFailureThreshold)

	// Next call must fail fast with ErrBrokenCircuit — no HTTP roundtrip.
	_, err := src.Fetch(context.Background())
	require.Error(t, err)
	assert.True(t, IsBreakerOpen(err),
		"expected breaker-open fast-fail; got %v", err)
	assert.True(t, errors.Is(err, breaker.ErrBrokenCircuit),
		"errors.Is(err, breaker.ErrBrokenCircuit) must be true")
}

func TestSourceBreaker_HalfOpenRecovers(t *testing.T) {
	var serveErrors int
	mu := struct {
		flip bool
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if mu.flip {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(minimalFixture))
			return
		}
		serveErrors++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	resetDelay := 50 * time.Millisecond
	b := newPrivateSourceBreaker(t, resetDelay)
	src := &ModelsDevSource{URL: srv.URL, Breaker: b}

	// Trip the breaker.
	for i := 0; i < sourceBreakerFailureThreshold; i++ {
		_, err := src.Fetch(context.Background())
		require.Errorf(t, err, "iter %d", i)
	}
	require.Equal(t, breaker.Open, b.State(), "breaker should be Open")

	// Fast-fail confirmation while Open.
	_, err := src.Fetch(context.Background())
	require.Error(t, err)
	require.True(t, IsBreakerOpen(err))

	// Flip the upstream to healthy and wait past the reset delay.
	mu.flip = true
	time.Sleep(resetDelay + 50*time.Millisecond)

	// SuccessThreshold=1: a single HalfOpen probe success closes the
	// breaker again.
	_, err = src.Fetch(context.Background())
	require.NoError(t, err, "first probe after reset delay must succeed")
	assert.Equal(t, breaker.Closed, b.State(),
		"breaker must close after a successful HalfOpen probe")

	// Subsequent calls remain healthy.
	_, err = src.Fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, breaker.Closed, b.State())
}

func TestSourceBreaker_DefaultBreakerSingleton(t *testing.T) {
	// SourceBreaker is process-wide and must return the same instance
	// across calls. Smoke-test the registration shape so adopters that
	// rely on breaker.Lookup(aim.SourceBreakerName) succeed.
	first := SourceBreaker()
	second := SourceBreaker()
	require.Same(t, first, second, "SourceBreaker must memoize")
	assert.Equal(t, SourceBreakerName, first.Name())

	got, ok := breaker.Lookup(SourceBreakerName)
	require.True(t, ok, "SourceBreaker should be visible to breaker.Lookup")
	assert.Same(t, first, got)
}
