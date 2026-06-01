package conformance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"hop.top/aim"
	"hop.top/kit/go/core/breaker"
)

// TestFactor10Delegation verifies the Factor 10 contract: the source
// fetcher is guarded by a circuit breaker; after the failure threshold
// the breaker trips Open and subsequent fetches fail fast (no 30s
// timeout); `aim status` reflects the live breaker state.
//
// Spec ref: 12-factor AI-CLI §10 — Resilient Delegation.
// Implementation: aim.SourceBreaker + ModelsDevSource.Fetch breaker
// guard; internal/status/status.go SourceBreaker provider.
func TestFactor10Delegation(t *testing.T) {
	// Stand up an always-500 upstream so every fetch fails. Long reset
	// delay keeps the breaker Open for the duration of the test.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	// Use a private breaker so we don't pollute the process-wide one
	// (other tests rely on a clean breaker). Pinned thresholds match
	// the production tunings the spec contracts on.
	name := "conformance-source-breaker"
	t.Cleanup(func() { breaker.Unregister(name) })
	b := breaker.New(name, breaker.WithCircuit(breaker.CircuitOpts{
		FailureThreshold: 3,
		SuccessThreshold: 1,
		Delay:            10 * 1000_000_000, // 10s
	}))
	src := &aim.ModelsDevSource{URL: srv.URL, Breaker: b}

	// Three failures trip the breaker to Open.
	for i := 0; i < 3; i++ {
		_, err := src.Fetch(context.Background())
		if err == nil {
			t.Fatalf("iter %d expected upstream failure", i)
		}
	}
	if b.State() != breaker.Open {
		t.Fatalf("breaker state = %v, want Open after threshold", b.State())
	}

	// Next call must fail fast with ErrBrokenCircuit (no HTTP round
	// trip — no 30 s timeout drain).
	_, err := src.Fetch(context.Background())
	if err == nil || !errors.Is(err, breaker.ErrBrokenCircuit) {
		t.Errorf("fast-fail expected; got %v", err)
	}
	if !aim.IsBreakerOpen(err) {
		t.Errorf("IsBreakerOpen(err) = false; want true")
	}

	// Trip the process-wide breaker too so `aim status` reflects an
	// Open state. The aim status provider reads the process-wide one
	// (aim.SourceBreakerName) via breaker.Lookup. Trip it by running
	// three fetches through the registered breaker.
	procB := aim.SourceBreaker()
	// Force-trip via the executor by feeding it three error outcomes.
	for i := 0; i < 3; i++ {
		_, _ = procB.Executor().Get(func() (any, error) {
			return nil, errors.New("conformance: simulated upstream 5xx")
		})
	}
	if procB.State() != breaker.Open {
		t.Fatalf("process breaker state = %v, want Open", procB.State())
	}
	// Reset the process-wide breaker on cleanup so unrelated tests in
	// the same run don't see a tripped fuse.
	t.Cleanup(func() { procB.Reset() })

	// `aim status --format json` must show source-breaker.state == "open".
	primeXDGCache(t)
	root := newRoot(t)
	stdout, stderr, errStatus := runCmd(t, root, "status", "--format", "json")
	if errStatus != nil {
		t.Fatalf("aim status failed: %v\nstderr: %s", errStatus, stderr)
	}
	var out struct {
		Sections []struct {
			Title string         `json:"title"`
			Data  map[string]any `json:"data"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	var brokerState string
	for _, s := range out.Sections {
		if s.Title == "source-breaker" {
			brokerState, _ = s.Data["state"].(string)
		}
	}
	if brokerState != "open" {
		t.Errorf("source-breaker.state = %q, want \"open\"", brokerState)
	}
}
