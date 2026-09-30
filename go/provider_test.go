package aim

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// providerFactsVector is one entry in spec/fixtures/provider-facts-vectors.json.
type providerFactsVector struct {
	Description string `json:"description"`
	ID          string `json:"id"`
	Expected    struct {
		Aliases  []string `json:"aliases"`
		KeyVars  []string `json:"key_vars"`
		Settings []string `json:"settings"`
		Local    bool     `json:"local"`
		Protocol string   `json:"protocol"`
		BaseURL  string   `json:"base_url"`
	} `json:"expected"`
}

// providerLookupVector is one entry in spec/fixtures/provider-lookup-vectors.json.
type providerLookupVector struct {
	Description string `json:"description"`
	Name        string `json:"name"`
	Found       bool   `json:"found"`
	ExpectedID  string `json:"expected_id"`
}

func loadJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, v))
}

func providerFixture(t *testing.T) map[string]*Provider {
	t.Helper()
	var providers map[string]*Provider
	loadJSON(t, "../spec/fixtures/provider-fixture.json", &providers)
	require.NotEmpty(t, providers)
	for key, p := range providers {
		require.Equal(t, key, p.ID, "fixture map key must equal provider id")
	}
	return providers
}

// providerRegistry builds a Registry over the provider fixture with a
// per-test cache dir, so neither the network nor the user's cache is read.
func providerRegistry(t *testing.T) *Registry {
	t.Helper()
	return NewRegistry(
		WithSource(newStaticSource(providerFixture(t))),
		WithCacheOpts(WithCacheDir(t.TempDir())),
	)
}

// orEmpty maps nil to an empty slice so JSON [] compares equal to a nil result.
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func TestProviderFacts_Vectors(t *testing.T) {
	providers := providerFixture(t)
	var vectors []providerFactsVector
	loadJSON(t, "../spec/fixtures/provider-facts-vectors.json", &vectors)
	require.Len(t, vectors, len(providers), "one facts vector per fixture provider")

	for _, v := range vectors {
		t.Run(v.Description, func(t *testing.T) {
			p, ok := providers[v.ID]
			require.True(t, ok, "fixture has no provider %q", v.ID)

			assert.Equal(t, orEmpty(v.Expected.Aliases), orEmpty(p.Aliases()), "aliases")
			assert.Equal(t, orEmpty(v.Expected.KeyVars), orEmpty(p.KeyVars()), "key vars")
			assert.Equal(t, orEmpty(v.Expected.Settings), orEmpty(p.Settings()), "settings")
			assert.Equal(t, v.Expected.Local, p.IsLocal(), "local")
			assert.Equal(t, v.Expected.Protocol, p.Protocol(), "protocol")
			assert.Equal(t, v.Expected.BaseURL, p.API, "base url")
		})
	}
}

func TestRegistryProvider_LookupVectors(t *testing.T) {
	reg := providerRegistry(t)
	var vectors []providerLookupVector
	loadJSON(t, "../spec/fixtures/provider-lookup-vectors.json", &vectors)
	require.NotEmpty(t, vectors)

	for _, v := range vectors {
		t.Run(v.Description, func(t *testing.T) {
			p, ok, err := reg.Provider(context.Background(), v.Name)
			require.NoError(t, err)
			require.Equal(t, v.Found, ok)
			assert.Equal(t, v.ExpectedID, p.ID)
		})
	}
}

// TestRegistryProvider_FactsViaRegistry: facts on a looked-up value match
// facts on the catalog value, whichever name was used.
func TestRegistryProvider_FactsViaRegistry(t *testing.T) {
	reg := providerRegistry(t)
	ctx := context.Background()

	byID, ok, err := reg.Provider(ctx, "google")
	require.NoError(t, err)
	require.True(t, ok)
	byAlias, ok, err := reg.Provider(ctx, "gemini")
	require.NoError(t, err)
	require.True(t, ok)

	assert.Equal(t, byID.ID, byAlias.ID)
	assert.Equal(t, byID.KeyVars(), byAlias.KeyVars())
	assert.Equal(t, []string{"GOOGLE_API_KEY", "GOOGLE_GENERATIVE_AI_API_KEY", "GEMINI_API_KEY"}, byAlias.KeyVars())
}

// TestRegistryProvider_CatalogIDWinsOverAlias: if the catalog ever lists a
// provider under a name the overlay uses as an alias, the real provider wins.
func TestRegistryProvider_CatalogIDWinsOverAlias(t *testing.T) {
	providers := providerFixture(t)
	providers["gemini"] = &Provider{ID: "gemini", Name: "Gemini (hypothetical)", Models: map[string]*Model{}}
	reg := NewRegistry(
		WithSource(newStaticSource(providers)),
		WithCacheOpts(WithCacheDir(t.TempDir())),
	)
	p, ok, err := reg.Provider(context.Background(), "gemini")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "gemini", p.ID)
}

func TestCanonicalProviderID_LookupVectors(t *testing.T) {
	var vectors []providerLookupVector
	loadJSON(t, "../spec/fixtures/provider-lookup-vectors.json", &vectors)
	for _, v := range vectors {
		t.Run(v.Description, func(t *testing.T) {
			want := v.Name // unknown names come back unchanged
			if v.Found {
				want = v.ExpectedID
			}
			assert.Equal(t, want, CanonicalProviderID(v.Name))
		})
	}
}

// TestProviderAliases_NoCollision: an alias must never shadow a real
// provider id, nor another alias, nor map to itself.
func TestProviderAliases_NoCollision(t *testing.T) {
	providers := providerFixture(t)
	seen := map[string]string{}
	for alias, id := range providerAliases {
		assert.NotEqual(t, alias, id, "alias %q maps to itself", alias)
		_, clash := providers[alias]
		assert.False(t, clash, "alias %q collides with fixture provider id", alias)
		_, target := providerAliases[id]
		assert.False(t, target, "alias %q targets %q, itself an alias", alias, id)
		_, known := providers[id]
		assert.True(t, known, "alias %q targets %q, absent from the fixture", alias, id)
		if prev, dup := seen[alias]; dup {
			t.Errorf("alias %q listed twice (%q, %q)", alias, prev, id)
		}
		seen[alias] = id
	}
}

func TestProviderAliases_AcceptsAliasOrID(t *testing.T) {
	assert.Equal(t, []string{"gemini"}, ProviderAliases("google"))
	assert.Equal(t, []string{"gemini"}, ProviderAliases("gemini"))
	assert.Empty(t, ProviderAliases("deepseek"))
	assert.Empty(t, ProviderAliases("nope"))

	// Callers get a copy; mutating it must not corrupt the table.
	got := ProviderAliases("google")
	got[0] = "mutated"
	assert.Equal(t, []string{"gemini"}, ProviderAliases("google"))
}

// TestProviderFacts_SlicesAreCopies: KeyVars/Settings never alias p.Env.
func TestProviderFacts_SlicesAreCopies(t *testing.T) {
	p := Provider{ID: "x", Env: []string{"X_API_KEY", "X_REGION"}}
	keys, settings := p.KeyVars(), p.Settings()
	keys[0], settings[0] = "mutated", "mutated"
	assert.Equal(t, []string{"X_API_KEY", "X_REGION"}, p.Env)
}

// providerProtocolVector is one entry in spec/fixtures/provider-protocol-vectors.json.
type providerProtocolVector struct {
	Description string `json:"description"`
	NPM         string `json:"npm"`
	Expected    string `json:"expected"`
}

func TestProviderProtocol_Vectors(t *testing.T) {
	var vectors []providerProtocolVector
	loadJSON(t, "../spec/fixtures/provider-protocol-vectors.json", &vectors)
	covered := map[string]bool{}
	for _, v := range vectors {
		covered[v.NPM] = true
		t.Run(v.Description, func(t *testing.T) {
			assert.Equal(t, v.Expected, Provider{NPM: v.NPM}.Protocol())
		})
	}
	// Every mapped package is pinned, so the ports cannot drift.
	for npm := range npmProtocols {
		assert.True(t, covered[npm], "npm %q has no protocol vector", npm)
	}
}

// TestProviderAliases_AllPinned: every alias has a lookup vector, so the
// ports carry the same overlay.
func TestProviderAliases_AllPinned(t *testing.T) {
	var vectors []providerLookupVector
	loadJSON(t, "../spec/fixtures/provider-lookup-vectors.json", &vectors)
	pinned := map[string]string{}
	for _, v := range vectors {
		if v.Found && v.Name != v.ExpectedID {
			pinned[v.Name] = v.ExpectedID
		}
	}
	assert.Equal(t, providerAliases, pinned)
}
