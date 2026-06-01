package aim

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimalFixture = `{
  "openai": {
    "id": "openai",
    "name": "OpenAI",
    "models": {
      "gpt-4o": {
        "id": "gpt-4o",
        "name": "GPT-4o",
        "modalities": {"input": ["text", "image"], "output": ["text"]},
        "tool_call": true,
        "limit": {"context": 128000}
      }
    }
  }
}`

func TestSourceFetch_Minimal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(minimalFixture))
	}))
	defer srv.Close()

	src := &ModelsDevSource{URL: srv.URL, Breaker: newPrivateSourceBreaker(t, time.Hour)}
	providers, err := src.Fetch(context.Background())
	require.NoError(t, err)
	require.NotNil(t, providers)

	p, ok := providers["openai"]
	require.True(t, ok, "expected openai provider")
	assert.Equal(t, "openai", p.ID)
	assert.Equal(t, "OpenAI", p.Name)
	require.NotNil(t, p.Models["gpt-4o"])
	assert.Equal(t, "openai", p.Models["gpt-4o"].Provider)
	assert.True(t, p.Models["gpt-4o"].ToolCall)
}

func TestSourceFetch_ForwardCompat_UnknownFields(t *testing.T) {
	fixture := `{
  "acme": {
    "id": "acme",
    "name": "Acme",
    "future_field": "should be ignored",
    "models": {
      "m1": {
        "id": "m1",
        "name": "Model 1",
        "unknown_future_cap": true,
        "modalities": {"input": ["text"], "output": ["text"]},
        "limit": {}
      }
    }
  }
}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer srv.Close()

	src := &ModelsDevSource{URL: srv.URL, Breaker: newPrivateSourceBreaker(t, time.Hour)}
	providers, err := src.Fetch(context.Background())
	require.NoError(t, err)
	require.NotNil(t, providers["acme"])
	assert.Equal(t, "m1", providers["acme"].Models["m1"].ID)
}

func TestSourceFetch_MaxResponseSize_Rejected(t *testing.T) {
	// Server returns body larger than MaxResponseSize.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write 12 bytes — exceeds MaxResponseSize of 10.
		_, _ = w.Write([]byte(strings.Repeat("x", 12)))
	}))
	defer srv.Close()

	src := &ModelsDevSource{URL: srv.URL, MaxResponseSize: 10, Breaker: newPrivateSourceBreaker(t, time.Hour)}
	_, err := src.Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds max size")
}

func TestSourceFetch_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	src := &ModelsDevSource{URL: srv.URL, Breaker: newPrivateSourceBreaker(t, time.Hour)}
	_, err := src.Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestSourceFetch_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := &ModelsDevSource{URL: srv.URL, Breaker: newPrivateSourceBreaker(t, time.Hour)}
	_, err := src.Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestSourceFetch_KeyMismatch(t *testing.T) {
	// Provider map key "foo" but Provider.ID is "bar".
	fixture := `{
  "foo": {
    "id": "bar",
    "name": "Bar",
    "models": {}
  }
}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer srv.Close()

	src := &ModelsDevSource{URL: srv.URL, Breaker: newPrivateSourceBreaker(t, time.Hour)}
	_, err := src.Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo")
	assert.Contains(t, err.Error(), "bar")
}

func TestSourceFetch_EmptyIDInheritsKey(t *testing.T) {
	// Provider.ID is empty — should be backfilled from map key.
	fixture := `{
  "myco": {
    "name": "MyCo",
    "models": {}
  }
}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer srv.Close()

	src := &ModelsDevSource{URL: srv.URL, Breaker: newPrivateSourceBreaker(t, time.Hour)}
	providers, err := src.Fetch(context.Background())
	require.NoError(t, err)
	require.NotNil(t, providers["myco"])
	assert.Equal(t, "myco", providers["myco"].ID)
}
