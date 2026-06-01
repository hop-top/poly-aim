package aim

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// queryVector is one entry in testdata/query-vectors.json.
type queryVector struct {
	Description string          `json:"description"`
	Input       string          `json:"input"`
	Error       bool            `json:"error"`
	Expected    json.RawMessage `json:"expected"`
}

// expectedFields holds all optional expected fields from the vector.
type expectedFields struct {
	Input            []string `json:"Input"`
	Output           []string `json:"Output"`
	Provider         string   `json:"Provider"`
	Family           string   `json:"Family"`
	Query            string   `json:"Query"`
	ToolCall         *bool    `json:"ToolCall"`
	Reasoning        *bool    `json:"Reasoning"`
	OpenWeights      *bool    `json:"OpenWeights"`
	StructuredOutput *bool    `json:"StructuredOutput"`
	Temperature      *bool    `json:"Temperature"`
}

func TestParseQuery_Vectors(t *testing.T) {
	b, err := os.ReadFile("testdata/query-vectors.json")
	require.NoError(t, err)

	var vectors []queryVector
	require.NoError(t, json.Unmarshal(b, &vectors))
	require.NotEmpty(t, vectors, "expected at least one test vector")

	for _, v := range vectors {
		v := v
		t.Run(v.Description, func(t *testing.T) {
			f, err := ParseQuery(v.Input)
			if v.Error {
				require.Error(t, err, "expected parse error for %q", v.Input)
				return
			}
			require.NoError(t, err)

			var exp expectedFields
			if len(v.Expected) > 0 && string(v.Expected) != "{}" {
				require.NoError(t, json.Unmarshal(v.Expected, &exp))
			}

			if len(exp.Input) > 0 {
				assert.Equal(t, exp.Input, f.Input, "Input mismatch")
			} else {
				assert.Empty(t, f.Input)
			}
			if len(exp.Output) > 0 {
				assert.Equal(t, exp.Output, f.Output, "Output mismatch")
			} else {
				assert.Empty(t, f.Output)
			}
			if exp.Provider != "" {
				assert.Equal(t, exp.Provider, f.Provider, "Provider mismatch")
			} else {
				assert.Empty(t, f.Provider)
			}
			if exp.Family != "" {
				assert.Equal(t, exp.Family, f.Family, "Family mismatch")
			} else {
				assert.Empty(t, f.Family)
			}
			if exp.Query != "" {
				assert.Equal(t, exp.Query, f.Query, "Query mismatch")
			} else {
				assert.Empty(t, f.Query)
			}
			if exp.ToolCall != nil {
				require.NotNil(t, f.ToolCall, "ToolCall should be set")
				assert.Equal(t, *exp.ToolCall, *f.ToolCall, "ToolCall mismatch")
			} else {
				assert.Nil(t, f.ToolCall)
			}
			if exp.Reasoning != nil {
				require.NotNil(t, f.Reasoning, "Reasoning should be set")
				assert.Equal(t, *exp.Reasoning, *f.Reasoning, "Reasoning mismatch")
			} else {
				assert.Nil(t, f.Reasoning)
			}
			if exp.OpenWeights != nil {
				require.NotNil(t, f.OpenWeights, "OpenWeights should be set")
				assert.Equal(t, *exp.OpenWeights, *f.OpenWeights, "OpenWeights mismatch")
			} else {
				assert.Nil(t, f.OpenWeights)
			}
			if exp.StructuredOutput != nil {
				require.NotNil(t, f.StructuredOutput, "StructuredOutput should be set")
				assert.Equal(t, *exp.StructuredOutput, *f.StructuredOutput, "StructuredOutput mismatch")
			} else {
				assert.Nil(t, f.StructuredOutput)
			}
			if exp.Temperature != nil {
				require.NotNil(t, f.Temperature, "Temperature should be set")
				assert.Equal(t, *exp.Temperature, *f.Temperature, "Temperature mismatch")
			} else {
				assert.Nil(t, f.Temperature)
			}
		})
	}
}

// TestQuery_RegistryRoundTrip builds a Registry with static data and calls Query.
func TestQuery_RegistryRoundTrip(t *testing.T) {
	data := map[string]*Provider{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]*Model{
				"gpt-4o": {
					ID:       "gpt-4o",
					Name:     "GPT-4o",
					Provider: "openai",
					ToolCall: true,
					Modalities: Modalities{
						Input:  []string{"text", "image"},
						Output: []string{"text"},
					},
				},
				"gpt-4": {
					ID:       "gpt-4",
					Name:     "GPT-4",
					Provider: "openai",
					ToolCall: true,
					Modalities: Modalities{
						Input:  []string{"text"},
						Output: []string{"text"},
					},
				},
			},
		},
		"anthropic": {
			ID:   "anthropic",
			Name: "Anthropic",
			Models: map[string]*Model{
				"claude-3-5": {
					ID:       "claude-3-5",
					Name:     "Claude 3.5",
					Provider: "anthropic",
					ToolCall: true,
					Modalities: Modalities{
						Input:  []string{"text", "image"},
						Output: []string{"text"},
					},
				},
			},
		},
	}

	src := newStaticSource(data)
	reg := NewRegistry(
		WithSource(src),
		WithCacheOpts(WithCacheDir(t.TempDir()), WithTTL(0)),
	)

	// Query by provider.
	models, err := reg.Query(context.Background(), "provider:openai")
	require.NoError(t, err)
	assert.Len(t, models, 2, "expected 2 openai models")
	for _, m := range models {
		assert.Equal(t, "openai", m.Provider)
	}

	// Query by input modality.
	models, err = reg.Query(context.Background(), "in:image")
	require.NoError(t, err)
	assert.Len(t, models, 2, "expected 2 image-input models")

	// Query by free text.
	models, err = reg.Query(context.Background(), "gpt-4o")
	require.NoError(t, err)
	assert.Len(t, models, 1)
	assert.Equal(t, "gpt-4o", models[0].ID)

	// Query with tool_call:true.
	models, err = reg.Query(context.Background(), "tool_call:true")
	require.NoError(t, err)
	assert.Len(t, models, 3)

	// Query with invalid syntax returns error.
	_, err = reg.Query(context.Background(), "tool_call:maybe")
	require.Error(t, err)
}
