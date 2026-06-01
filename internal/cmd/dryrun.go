package cmd

import (
	"fmt"
	"io"
	"time"

	"hop.top/aim"
	"hop.top/kit/go/console/output"
)

// aimFilter is a local alias to keep the filterToMap signature short.
type aimFilter = aim.Filter

// envelopeWithWarnings is the wire shape used when a leaf needs to
// surface a deprecation or non-fatal advisory alongside the standard
// {data, _meta} envelope. We assemble it manually because kit's
// renderer only knows the two-slot envelope.
type envelopeWithWarnings struct {
	Data     any              `json:"data"     yaml:"data"`
	Meta     *output.Metadata `json:"_meta"    yaml:"_meta"`
	Warnings []string         `json:"warnings" yaml:"warnings"`
}

// RefreshPreview is the structured payload emitted by `refresh --dry-run`.
// Fields are populated from cache metadata; no network call is made.
//
// `Status` is either "would_refresh" or "would_skip". `Reason` explains
// the planner's decision: "ttl_expired", "ttl_remaining", "force",
// or "no_prior_fetch".
type RefreshPreview struct {
	Status            string `json:"status"                        yaml:"status"`
	Reason            string `json:"reason"                        yaml:"reason"`
	WouldFetchURL     string `json:"would_fetch_url"               yaml:"would_fetch_url"`
	WouldWritePaths   []string `json:"would_write_paths"           yaml:"would_write_paths"`
	CurrentETag       string `json:"current_etag,omitempty"        yaml:"current_etag,omitempty"`
	CurrentLastFetch  string `json:"current_last_fetch,omitempty"  yaml:"current_last_fetch,omitempty"`
	CurrentAge        string `json:"current_age,omitempty"         yaml:"current_age,omitempty"`
	TTLRemaining      string `json:"ttl_remaining,omitempty"       yaml:"ttl_remaining,omitempty"`
	WouldSkipDueToTTL bool   `json:"would_skip_due_to_ttl"         yaml:"would_skip_due_to_ttl"`
}

// QueryExplain is the structured payload emitted by `aim query --explain`.
// It surfaces the parsed AST without running the query so agents can
// validate DSL shape before executing.
type QueryExplain struct {
	ExprInput string         `json:"expr_input"            yaml:"expr_input"`
	Filter    map[string]any `json:"filter"                yaml:"filter"`
	FreeText  []string       `json:"free_text,omitempty"   yaml:"free_text,omitempty"`
}

// filterToMap projects an aim.Filter into a wire-friendly map. Only
// non-zero fields are included; tristate booleans are encoded as bool
// values (omitted when nil).
func filterToMap(f aimFilter) map[string]any {
	m := map[string]any{}
	if len(f.Input) > 0 {
		m["input"] = f.Input
	}
	if len(f.Output) > 0 {
		m["output"] = f.Output
	}
	if f.Provider != "" {
		m["provider"] = f.Provider
	}
	if f.Family != "" {
		m["family"] = f.Family
	}
	if f.ToolCall != nil {
		m["tool_call"] = *f.ToolCall
	}
	if f.Reasoning != nil {
		m["reasoning"] = *f.Reasoning
	}
	if f.OpenWeights != nil {
		m["open_weights"] = *f.OpenWeights
	}
	if f.Query != "" {
		m["query"] = f.Query
	}
	return m
}

// renderWithWarnings writes the standard {data, _meta} envelope and
// folds a top-level `warnings` array in for JSON/YAML formats. For
// table mode, the warnings are written one-per-line to stderr after the
// payload but before the provenance footer (which is still emitted by
// kit's renderer or the caller's bespoke table path).
//
// Adopters that have no warnings should keep calling output.Render
// directly so the envelope stays minimal.
func renderWithWarnings(
	stdout, stderr io.Writer,
	format output.Format,
	payload any,
	meta output.Metadata,
	warnings []string,
) error {
	if len(warnings) == 0 {
		return output.Render(stdout, format, payload, output.WithProvenance(meta))
	}

	switch format {
	case output.JSON, output.YAML:
		// Render the full envelope ourselves so the warnings array
		// lives at the top level alongside data/_meta. Kit's
		// WithProvenance wrapper is bypassed here because it only
		// emits {data, _meta} and we need a third slot.
		env := envelopeWithWarnings{
			Data:     payload,
			Meta:     &meta,
			Warnings: warnings,
		}
		return output.Render(stdout, format, env)
	default:
		// Table / text / other: write the payload via kit's renderer
		// (with the provenance footer to stderr); follow with one
		// stderr line per warning so operators see the deprecation.
		if err := output.Render(stdout, format, payload, output.WithProvenance(meta)); err != nil {
			return err
		}
		for _, w := range warnings {
			_, _ = fmt.Fprintf(stderr, "warning: %s\n", w)
		}
		return nil
	}
}

// formatDuration renders a duration as a kebab-friendly RFC3339-adjacent
// human form. We avoid time.Duration.String for the spec — agents prefer
// a stable lower-case form.
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	return d.Truncate(time.Second).String()
}
