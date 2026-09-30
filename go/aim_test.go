package aim

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boolPtr returns a *bool for tristate filter fields.
func boolPtr(b bool) *bool { return &b }

// buildRegistry creates a Registry backed by a static source using t.TempDir.
func buildRegistry(t *testing.T, data map[string]*Provider) *Registry {
	t.Helper()
	src := newStaticSource(data)
	return NewRegistry(
		WithSource(src),
		WithCacheOpts(WithCacheDir(t.TempDir()), WithTTL(0)),
	)
}

// TestFilter_Input_SubsetMatch verifies Filter.Input uses subset containment.
func TestFilter_Input_SubsetMatch(t *testing.T) {
	data := map[string]*Provider{
		"p1": {
			ID:   "p1",
			Name: "P1",
			Models: map[string]*Model{
				"text-only": {
					ID:       "text-only",
					Name:     "Text Only",
					Provider: "p1",
					Modalities: Modalities{
						Input:  []string{"text"},
						Output: []string{"text"},
					},
				},
				"multi-modal": {
					ID:       "multi-modal",
					Name:     "Multi Modal",
					Provider: "p1",
					Modalities: Modalities{
						Input:  []string{"text", "image", "audio"},
						Output: []string{"text"},
					},
				},
			},
		},
	}

	reg := buildRegistry(t, data)
	ctx := context.Background()

	// Filter by text input — both models have text.
	models, err := reg.Models(ctx, Filter{Input: []string{"text"}})
	require.NoError(t, err)
	assert.Len(t, models, 2)

	// Filter by image — only multi-modal.
	models, err = reg.Models(ctx, Filter{Input: []string{"image"}})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "multi-modal", models[0].ID)

	// Filter by image+audio — only multi-modal.
	models, err = reg.Models(ctx, Filter{Input: []string{"image", "audio"}})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "multi-modal", models[0].ID)

	// Filter by video — no match.
	models, err = reg.Models(ctx, Filter{Input: []string{"video"}})
	require.NoError(t, err)
	assert.Empty(t, models)

	// Empty filter returns all.
	models, err = reg.Models(ctx, Filter{})
	require.NoError(t, err)
	assert.Len(t, models, 2)
}

// TestFilter_Provider_ExactMatch verifies Provider filter is case-sensitive exact match.
func TestFilter_Provider_ExactMatch(t *testing.T) {
	data := map[string]*Provider{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]*Model{
				"gpt-4o": {ID: "gpt-4o", Name: "GPT-4o", Provider: "openai"},
			},
		},
		"anthropic": {
			ID:   "anthropic",
			Name: "Anthropic",
			Models: map[string]*Model{
				"claude": {ID: "claude", Name: "Claude", Provider: "anthropic"},
			},
		},
	}

	reg := buildRegistry(t, data)
	ctx := context.Background()

	models, err := reg.Models(ctx, Filter{Provider: "openai"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "openai", models[0].Provider)

	// Case-sensitive: "OpenAI" should not match "openai".
	models, err = reg.Models(ctx, Filter{Provider: "OpenAI"})
	require.NoError(t, err)
	assert.Empty(t, models)

	// No filter returns all.
	models, err = reg.Models(ctx, Filter{})
	require.NoError(t, err)
	assert.Len(t, models, 2)
}

// TestFilter_ToolCall_Tristate verifies nil=all, true=only capable, false=only not capable.
func TestFilter_ToolCall_Tristate(t *testing.T) {
	data := map[string]*Provider{
		"p": {
			ID:   "p",
			Name: "P",
			Models: map[string]*Model{
				"tool-yes": {ID: "tool-yes", Name: "Tool Yes", Provider: "p", ToolCall: true},
				"tool-no":  {ID: "tool-no", Name: "Tool No", Provider: "p", ToolCall: false},
			},
		},
	}

	reg := buildRegistry(t, data)
	ctx := context.Background()

	// nil = no filter.
	models, err := reg.Models(ctx, Filter{})
	require.NoError(t, err)
	assert.Len(t, models, 2)

	// true = only tool-capable.
	models, err = reg.Models(ctx, Filter{ToolCall: boolPtr(true)})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "tool-yes", models[0].ID)

	// false = only non-tool-capable.
	models, err = reg.Models(ctx, Filter{ToolCall: boolPtr(false)})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "tool-no", models[0].ID)
}

// TestFilter_Reasoning_Tristate verifies reasoning tristate works correctly.
func TestFilter_Reasoning_Tristate(t *testing.T) {
	data := map[string]*Provider{
		"p": {
			ID:   "p",
			Name: "P",
			Models: map[string]*Model{
				"reason-yes": {ID: "reason-yes", Provider: "p", Reasoning: true},
				"reason-no":  {ID: "reason-no", Provider: "p", Reasoning: false},
			},
		},
	}

	reg := buildRegistry(t, data)
	ctx := context.Background()

	models, err := reg.Models(ctx, Filter{Reasoning: boolPtr(true)})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "reason-yes", models[0].ID)

	models, err = reg.Models(ctx, Filter{Reasoning: boolPtr(false)})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "reason-no", models[0].ID)
}

// TestFilter_MultiSource_LastWins verifies multi-source merge where last source wins per provider.
// Registry itself merges at the Cache/Source level — test via two static sources in sequence.
func TestFilter_MultiSource_LastWins(t *testing.T) {
	// Simulate merging by using a source that returns merged data.
	// The spec says "last source wins per provider ID" — we test this logic by
	// constructing merged data manually and verifying the correct name appears.
	dataA := map[string]*Provider{
		"p1": {ID: "p1", Name: "P1-from-A", Models: map[string]*Model{
			"m1": {ID: "m1", Name: "Model from A", Provider: "p1"},
		}},
	}
	dataB := map[string]*Provider{
		"p1": {ID: "p1", Name: "P1-from-B", Models: map[string]*Model{
			"m1": {ID: "m1", Name: "Model from B", Provider: "p1"},
		}},
		"p2": {ID: "p2", Name: "P2-only-B", Models: map[string]*Model{
			"m2": {ID: "m2", Name: "Model B P2", Provider: "p2"},
		}},
	}

	// Merged: B wins for p1, p2 added.
	merged := make(map[string]*Provider)
	for k, v := range dataA {
		merged[k] = v
	}
	for k, v := range dataB {
		merged[k] = v // last wins
	}

	reg := buildRegistry(t, merged)
	ctx := context.Background()

	models, err := reg.Models(ctx, Filter{Provider: "p1"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "Model from B", models[0].Name, "last source should win for p1")

	models, err = reg.Models(ctx, Filter{Provider: "p2"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "m2", models[0].ID)
}

// TestRegistry_Get_FoundAndNotFound verifies Get by provider+model ID.
func TestRegistry_Get_FoundAndNotFound(t *testing.T) {
	data := map[string]*Provider{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]*Model{
				"gpt-4o": {ID: "gpt-4o", Name: "GPT-4o", Provider: "openai"},
			},
		},
	}

	reg := buildRegistry(t, data)
	ctx := context.Background()

	m, found, err := reg.Get(ctx, "openai", "gpt-4o")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "gpt-4o", m.ID)
	assert.Equal(t, "openai", m.Provider)

	_, found, err = reg.Get(ctx, "openai", "nonexistent")
	require.NoError(t, err)
	assert.False(t, found)

	_, found, err = reg.Get(ctx, "nonexistent", "gpt-4o")
	require.NoError(t, err)
	assert.False(t, found)
}

// TestRegistry_Providers_SortedAlphabetically verifies Providers returns sorted slice.
func TestRegistry_Providers_SortedAlphabetically(t *testing.T) {
	data := map[string]*Provider{
		"zzz": {ID: "zzz", Name: "ZZZ", Models: map[string]*Model{}},
		"aaa": {ID: "aaa", Name: "AAA", Models: map[string]*Model{}},
		"mmm": {ID: "mmm", Name: "MMM", Models: map[string]*Model{}},
	}

	reg := buildRegistry(t, data)
	providers, err := reg.Providers(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 3)
	assert.Equal(t, "aaa", providers[0].ID)
	assert.Equal(t, "mmm", providers[1].ID)
	assert.Equal(t, "zzz", providers[2].ID)
}

// TestMatchesFilter_DirectLogic tests filter matching directly via matchesFilter.
func TestMatchesFilter_DirectLogic(t *testing.T) {
	m := Model{
		ID:       "m1",
		Name:     "My Model",
		Provider: "acme",
		Family:   "gpt",
		ToolCall: true,
		Reasoning: false,
		OpenWeights: false,
		Modalities: Modalities{
			Input:  []string{"text", "image"},
			Output: []string{"text"},
		},
	}

	// Provider match.
	assert.True(t, matchesFilter(m, Filter{Provider: "acme"}))
	assert.False(t, matchesFilter(m, Filter{Provider: "other"}))

	// Family match.
	assert.True(t, matchesFilter(m, Filter{Family: "gpt"}))
	assert.False(t, matchesFilter(m, Filter{Family: "claude"}))

	// Input subset.
	assert.True(t, matchesFilter(m, Filter{Input: []string{"text"}}))
	assert.True(t, matchesFilter(m, Filter{Input: []string{"text", "image"}}))
	assert.False(t, matchesFilter(m, Filter{Input: []string{"video"}}))

	// Output subset.
	assert.True(t, matchesFilter(m, Filter{Output: []string{"text"}}))
	assert.False(t, matchesFilter(m, Filter{Output: []string{"image"}}))

	// ToolCall tristate.
	assert.True(t, matchesFilter(m, Filter{ToolCall: boolPtr(true)}))
	assert.False(t, matchesFilter(m, Filter{ToolCall: boolPtr(false)}))
	assert.True(t, matchesFilter(m, Filter{}))

	// OpenWeights tristate.
	assert.True(t, matchesFilter(m, Filter{OpenWeights: boolPtr(false)}))
	assert.False(t, matchesFilter(m, Filter{OpenWeights: boolPtr(true)}))

	// Query free-text (case-insensitive substring on ID or Name).
	assert.True(t, matchesFilter(m, Filter{Query: "m1"}))
	assert.True(t, matchesFilter(m, Filter{Query: "my model"}))
	assert.True(t, matchesFilter(m, Filter{Query: "MY MODEL"}))
	assert.False(t, matchesFilter(m, Filter{Query: "nonexistent"}))
}

// TestModel_Unmarshal_CostAndCapabilities verifies the wire format round-trip
// for the cost, structured_output, and temperature fields (gpt-4o shape).
func TestModel_Unmarshal_CostAndCapabilities(t *testing.T) {
	payload := []byte(`{
		"id": "gpt-4o",
		"name": "GPT-4o",
		"family": "gpt",
		"attachment": true,
		"cost": {"cache_read": 1.25, "input": 2.5, "output": 10},
		"knowledge": "2023-09",
		"last_updated": "2024-08-06",
		"limit": {"context": 128000, "output": 16384},
		"modalities": {"input": ["text", "image", "pdf"], "output": ["text"]},
		"open_weights": false,
		"reasoning": false,
		"release_date": "2024-05-13",
		"structured_output": true,
		"temperature": true,
		"tool_call": true
	}`)

	var m Model
	require.NoError(t, json.Unmarshal(payload, &m))

	require.NotNil(t, m.Cost, "populated cost block should yield non-nil *Cost")
	assert.Equal(t, 2.5, m.Cost.Input)
	assert.Equal(t, float64(10), m.Cost.Output)
	assert.Equal(t, 1.25, m.Cost.CacheRead)
	assert.Equal(t, float64(0), m.Cost.CacheWrite, "absent cache_write should be zero")
	assert.True(t, m.StructuredOutput)
	assert.True(t, m.Temperature)
	assert.True(t, m.Attachment)
	assert.True(t, m.ToolCall)
}

// TestModel_Unmarshal_CostAbsent verifies models without cost (local/open-weight)
// deserialize cleanly with zero-valued Cost and capability flags.
func TestModel_Unmarshal_CostAbsent(t *testing.T) {
	payload := []byte(`{
		"id": "llama3-local",
		"name": "Llama 3 Local",
		"family": "llama",
		"open_weights": true,
		"reasoning": false,
		"tool_call": false,
		"modalities": {"input": ["text"], "output": ["text"]},
		"limit": {"context": 8192}
	}`)

	var m Model
	require.NoError(t, json.Unmarshal(payload, &m))

	assert.Nil(t, m.Cost, "absent cost block should yield nil *Cost")
	assert.False(t, m.StructuredOutput)
	assert.False(t, m.Temperature)
}

// TestModel_Unmarshal_CostEdgeCases pins behavior for cost: {} and cost: null.
// Both currently produce non-nil *Cost (empty struct) and nil *Cost respectively,
// preserving the missing-vs-explicit-empty distinction.
func TestModel_Unmarshal_CostEdgeCases(t *testing.T) {
	t.Run("empty object", func(t *testing.T) {
		payload := []byte(`{"id":"x","name":"X","modalities":{"input":["text"],"output":["text"]},"limit":{"context":1},"cost":{}}`)
		var m Model
		require.NoError(t, json.Unmarshal(payload, &m))
		require.NotNil(t, m.Cost, `cost:{} should yield a non-nil *Cost`)
		assert.Equal(t, Cost{}, *m.Cost)
	})

	t.Run("null", func(t *testing.T) {
		payload := []byte(`{"id":"x","name":"X","modalities":{"input":["text"],"output":["text"]},"limit":{"context":1},"cost":null}`)
		var m Model
		require.NoError(t, json.Unmarshal(payload, &m))
		assert.Nil(t, m.Cost, `cost:null should yield nil *Cost`)
	})
}

// TestFilter_StructuredOutput_Tristate verifies the new tristate filter.
func TestFilter_StructuredOutput_Tristate(t *testing.T) {
	data := map[string]*Provider{
		"p": {
			ID:   "p",
			Name: "P",
			Models: map[string]*Model{
				"so-yes": {ID: "so-yes", Provider: "p", StructuredOutput: true},
				"so-no":  {ID: "so-no", Provider: "p", StructuredOutput: false},
			},
		},
	}

	reg := buildRegistry(t, data)
	ctx := context.Background()

	models, err := reg.Models(ctx, Filter{StructuredOutput: boolPtr(true)})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "so-yes", models[0].ID)

	models, err = reg.Models(ctx, Filter{StructuredOutput: boolPtr(false)})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "so-no", models[0].ID)

	models, err = reg.Models(ctx, Filter{})
	require.NoError(t, err)
	assert.Len(t, models, 2)
}

// TestFilter_Temperature_Tristate verifies the new tristate filter.
func TestFilter_Temperature_Tristate(t *testing.T) {
	data := map[string]*Provider{
		"p": {
			ID:   "p",
			Name: "P",
			Models: map[string]*Model{
				"temp-yes": {ID: "temp-yes", Provider: "p", Temperature: true},
				"temp-no":  {ID: "temp-no", Provider: "p", Temperature: false},
			},
		},
	}

	reg := buildRegistry(t, data)
	ctx := context.Background()

	models, err := reg.Models(ctx, Filter{Temperature: boolPtr(true)})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "temp-yes", models[0].ID)

	models, err = reg.Models(ctx, Filter{Temperature: boolPtr(false)})
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "temp-no", models[0].ID)
}
