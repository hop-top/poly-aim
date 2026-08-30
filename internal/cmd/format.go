// Package cmd implements the aim subcommands.
package cmd

import (
	"os"

	"github.com/mattn/go-isatty"
	"hop.top/kit/go/console/output"
)

// SchemaVersion is the aim CLI structured-output schema version.
// Bumps follow MAJOR.MINOR semantics; surface this through every leaf's
// kit/output-schema-version annotation so agents can negotiate.
const SchemaVersion = "aim/v1"

// refreshStatus is the structured payload emitted by `refresh`.
// Fields cover whether the cache was actually re-fetched, the next TTL
// boundary, and the source URL — enough for agents to chain follow-up
// reads without re-running `refresh` on a cache miss.
//
// The `table` tags are load-bearing beyond table mode: csv, text, and
// human all derive their columns from them, and an untagged struct
// renders as an empty document in each.
type refreshStatus struct {
	Refreshed   bool   `table:"Refreshed"    json:"refreshed"               yaml:"refreshed"`
	CachedUntil string `table:"Cached Until" json:"cached_until,omitempty"  yaml:"cached_until,omitempty"`
	Source      string `table:"Source"       json:"source,omitempty"        yaml:"source,omitempty"`
}

// defaultFormat returns "table" when stdout is a TTY, else "json".
func defaultFormat() output.Format {
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		return output.Table
	}
	return output.JSON
}

// boolPtr returns a pointer to b.
func boolPtr(b bool) *bool { return &b }
