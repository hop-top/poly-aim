// Package aim provides an AI model registry client backed by models.dev.
//
// Core types mirror the models.dev/api.json wire format 1:1.
// The registry supports programmatic [Filter] queries and a cross-language
// string query syntax (see [ParseQuery]).
//
// # Tristate booleans
//
// [Filter.ToolCall], [Filter.Reasoning], [Filter.OpenWeights],
// [Filter.StructuredOutput], and [Filter.Temperature] are *bool (tristate):
// nil means "don't filter", true/false means must match.
// Cross-language equivalents: TS boolean|undefined, Python Optional[bool].
//
// # Quick start
//
//	reg := aim.NewRegistry()
//	models, err := reg.Models(context.Background(), aim.Filter{Input: []string{"image"}})
package aim

import "context"

// Provider is the top-level entry in the models.dev registry.
// The map key in the wire format MUST equal Provider.ID.
type Provider struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Doc    string            `json:"doc,omitempty"`
	API    string            `json:"api,omitempty"`
	NPM    string            `json:"npm,omitempty"`
	Env    []string          `json:"env,omitempty"`
	Models map[string]*Model `json:"models"`
}

// Model is a single LLM entry within a provider.
// Provider field is populated from the parent map key during deserialization,
// not from the wire format itself.
//
// Cost values are USD per 1M tokens (sourced from models.dev). Many local
// open-weight models omit cost entirely; Cost is *Cost so absent (nil) is
// distinguishable from explicitly zero ({}).
type Model struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Family           string     `json:"family,omitempty"`
	Provider         string     `json:"-"` // populated from parent Provider.ID
	Modalities       Modalities `json:"modalities"`
	ToolCall         bool       `json:"tool_call"`
	Reasoning        bool       `json:"reasoning"`
	OpenWeights      bool       `json:"open_weights"`
	Attachment       bool       `json:"attachment,omitempty"`
	Cost             *Cost      `json:"cost,omitempty"`
	StructuredOutput bool       `json:"structured_output,omitempty"`
	Temperature      bool       `json:"temperature,omitempty"`
	ReleaseDate      string     `json:"release_date,omitempty"`
	LastUpdated      string     `json:"last_updated,omitempty"`
	Knowledge        string     `json:"knowledge,omitempty"`
	Limit            Limits     `json:"limit"`
}

// Modalities describes the input and output modalities a model supports.
type Modalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// Limits holds token/context window sizes for a model.
type Limits struct {
	Context int `json:"context,omitempty"`
	Input   int `json:"input,omitempty"`
	Output  int `json:"output,omitempty"`
}

// Cost holds per-token pricing for a model, expressed in USD per 1M tokens.
// All fields are optional; many open-weight/local models omit cost entirely.
type Cost struct {
	Input      float64 `json:"input,omitempty"`
	Output     float64 `json:"output,omitempty"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
}

// Filter constrains a model query. All non-zero fields are ANDed together.
//
// Input/Output check subset containment: Filter{Input: ["image"]} matches any
// model whose input modalities include "image".
//
// ToolCall, Reasoning, OpenWeights are tristates: nil = no filter.
//
// Query is a free-text string query parsed by [ParseQuery]; when set it is
// applied in addition to the other fields.
type Filter struct {
	// Input filters by required input modalities (subset match).
	Input []string
	// Output filters by required output modalities (subset match).
	Output []string
	// Provider filters by Provider.ID (exact match, case-sensitive).
	Provider string
	// Family filters by Model.Family (exact match, case-sensitive).
	Family string
	// ToolCall filters on tool-calling support.
	ToolCall *bool
	// Reasoning filters on reasoning/chain-of-thought support.
	Reasoning *bool
	// OpenWeights filters on open-weights availability.
	OpenWeights *bool
	// StructuredOutput filters on structured-output (JSON schema) support.
	StructuredOutput *bool
	// Temperature filters on whether the model accepts a temperature parameter.
	Temperature *bool
	// Query is an optional string query (see [ParseQuery]).
	Query string
}

// Source is the interface for fetching raw provider data.
// Implementations must return a map whose keys equal Provider.ID.
// Unknown JSON fields in the upstream payload MUST be silently ignored.
type Source interface {
	Fetch(ctx context.Context) (map[string]*Provider, error)
}
