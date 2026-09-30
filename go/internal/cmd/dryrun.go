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
//
// The `table` tags also drive the csv, text, and human formatters; an
// untagged struct renders as an empty document in all three.
type RefreshPreview struct {
	Status            string   `table:"Status"        json:"status"                        yaml:"status"`
	Reason            string   `table:"Reason"        json:"reason"                        yaml:"reason"`
	WouldFetchURL     string   `table:"Would Fetch"   json:"would_fetch_url"               yaml:"would_fetch_url"`
	WouldWritePaths   []string `table:"Would Write"   json:"would_write_paths"             yaml:"would_write_paths"`
	CurrentETag       string   `table:"ETag"          json:"current_etag,omitempty"        yaml:"current_etag,omitempty"`
	CurrentLastFetch  string   `table:"Last Fetch"    json:"current_last_fetch,omitempty"  yaml:"current_last_fetch,omitempty"`
	CurrentAge        string   `table:"Age"           json:"current_age,omitempty"         yaml:"current_age,omitempty"`
	TTLRemaining      string   `table:"TTL Remaining" json:"ttl_remaining,omitempty"       yaml:"ttl_remaining,omitempty"`
	WouldSkipDueToTTL bool     `table:"Skip (TTL)"    json:"would_skip_due_to_ttl"         yaml:"would_skip_due_to_ttl"`
}

// QueryExplain is the structured payload emitted by `aim query --explain`.
// It surfaces the parsed AST without running the query so agents can
// validate DSL shape before executing.
//
// The `table` tags also drive the csv, text, and human formatters; an
// untagged struct renders as an empty document in all three.
type QueryExplain struct {
	ExprInput string         `table:"Expression" json:"expr_input"            yaml:"expr_input"`
	Filter    map[string]any `table:"Filter"     json:"filter"                yaml:"filter"`
	FreeText  []string       `table:"Free Text"  json:"free_text,omitempty"   yaml:"free_text,omitempty"`
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
		return renderEnvelope(stdout, stderr, format, payload, meta)
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
		// Table / csv / text / human: write the payload via the
		// envelope helper (which routes provenance to stderr); follow
		// with one stderr line per warning so operators see the
		// deprecation.
		if err := renderEnvelope(stdout, stderr, format, payload, meta); err != nil {
			return err
		}
		for _, w := range warnings {
			_, _ = fmt.Fprintf(stderr, "warning: %s\n", w)
		}
		return nil
	}
}

// flatFormats are the tag-driven output formats other than table: they
// project a value through its `table:""` tags into a flat, columnar
// document with no place to nest a {data, _meta} envelope.
//
// Passing one of these through [output.WithProvenance] is silently
// destructive. Kit wraps the payload in an anonymous {Data, Meta} struct
// for every format except table, and that wrapper carries no `table`
// tags — so the csv/text/human formatters resolve zero columns and
// return early having written nothing at all. The command still exits 0,
// which for an agent audience is strictly worse than an error.
//
// These formats therefore render the payload unwrapped, with provenance
// emitted as a stderr footer instead — exactly the split kit already
// applies to table mode.
//
// Table itself is deliberately absent: kit already renders it unwrapped
// and emits its own footer, so adding it here would double the footer.
var flatFormats = map[output.Format]bool{
	output.CSV:   true,
	output.Text:  true,
	output.Human: true,
}

// renderEnvelope writes payload to stdout in format and guarantees
// provenance reaches the caller on every path.
//
// Structured formats (json, yaml) get the inline {data, _meta} envelope
// via kit's WithProvenance, and table gets kit's unwrapped render plus
// kit's own footer. Flat, tag-driven formats (see [flatFormats]) render
// the bare payload and receive an aim-emitted stderr footer, because
// wrapping them yields an empty document.
func renderEnvelope(
	stdout, stderr io.Writer,
	format output.Format,
	payload any,
	meta output.Metadata,
) error {
	if !flatFormats[format] {
		return output.Render(stdout, format, payload, output.WithProvenance(meta))
	}

	if err := renderFlat(stdout, format, payload); err != nil {
		return err
	}
	writeProvenanceFooter(stderr, meta)
	return nil
}

// renderFlat renders payload through the registered formatter for
// format, supplying that formatter's declared option defaults.
//
// This bypasses [output.Render] deliberately. That shim invokes
// Formatter.Render with a nil Options map, which is fine for formatters
// that have no options but fatal for csv: its delimiter default (",")
// lives in the OptionSpec, so a nil map yields an empty delimiter and
// every call fails with "delimiter must be exactly one character".
// Running the specs through ParseOptions with no user overrides
// materialises the declared defaults.
func renderFlat(stdout io.Writer, format output.Format, payload any) error {
	f, ok := output.Default.Lookup(format)
	if !ok {
		return fmt.Errorf("unknown output format %q", format)
	}
	opts, err := output.ParseOptions(nil, f.Options())
	if err != nil {
		return fmt.Errorf("output format %q: %w", format, err)
	}
	return f.Render(stdout, payload, opts, nil)
}

// writeProvenanceFooter emits the one-line stderr provenance summary
// used by the flat formats. The shape mirrors kit's own table-mode
// footer so operators see a consistent line regardless of format.
//
// Kit emits its footer only for table, so the other flat formats need
// one written here. Unlike kit's — which targets the package-level
// os.Stderr — this writes to the command's stderr, keeping the output
// capturable in tests and redirectable by callers.
func writeProvenanceFooter(stderr io.Writer, meta output.Metadata) {
	if stderr == nil {
		return
	}
	_, _ = fmt.Fprintf(stderr, "Source: %s (fetched %s, method=%s)\n",
		meta.Source,
		meta.FetchedAt.Format(time.RFC3339),
		meta.Method,
	)
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
