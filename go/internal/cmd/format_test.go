package cmd

import (
	"bytes"
	"context"
	"encoding/csv"
	"slices"
	"strings"
	"testing"
)

// declaredFormats is every value kit's registry accepts and that aim
// therefore advertises in `--format` help. Each one must produce output;
// a declared format that emits zero bytes and exits 0 is the worst
// possible outcome for the agent audience this CLI targets.
var declaredFormats = []string{"table", "json", "yaml", "csv", "text", "human"}

// TestAllDeclaredFormatsEmitOutput is the regression guard for the
// silent-empty-format bug: csv, text, and human were listed in help and
// accepted without complaint, but emitted 0 bytes and exited 0.
//
// Root cause was the provenance envelope — output.Render wraps the
// payload in an untagged {Data, Meta} struct for every format except
// table, and the csv/text/human formatters derive their columns from
// `table:""` tags. No tags on the wrapper meant no columns, and every
// one of them returns nil early on an empty column set.
func TestAllDeclaredFormatsEmitOutput(t *testing.T) {
	for _, format := range declaredFormats {
		t.Run(format, func(t *testing.T) {
			primeCache(t, fixturePayload(t))

			root := testRoot(t)
			root.Cmd.AddCommand(ListCmd(root))
			var stdout, stderr bytes.Buffer
			root.Cmd.SetOut(&stdout)
			root.Cmd.SetErr(&stderr)
			root.Cmd.SetArgs([]string{"list", "--format", format})

			if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("execute list --format %s: %v\nstderr: %s",
					format, err, stderr.String())
			}
			if stdout.Len() == 0 {
				t.Fatalf("--format %s emitted 0 bytes on stdout; "+
					"a declared format must never silently produce nothing",
					format)
			}
			// Every format must carry the actual data, not just a header
			// or a footer. The fixture has a known provider in it.
			if !strings.Contains(stdout.String(), "anthropic") {
				t.Fatalf("--format %s output missing expected payload content\n"+
					"got: %.400s", format, stdout.String())
			}
		})
	}
}

// TestTagDrivenFormatsCarryProvenance asserts that the formats which
// cannot embed the {data, _meta} envelope inline (csv, text, human are
// flat, tag-driven projections) still surface provenance on stderr, the
// same way table mode does. Factor 11 requires provenance reach the
// caller on every path; dropping the envelope must not drop provenance.
func TestTagDrivenFormatsCarryProvenance(t *testing.T) {
	for _, format := range []string{"csv", "text", "human"} {
		t.Run(format, func(t *testing.T) {
			primeCache(t, fixturePayload(t))

			root := testRoot(t)
			root.Cmd.AddCommand(ListCmd(root))
			var stdout, stderr bytes.Buffer
			root.Cmd.SetOut(&stdout)
			root.Cmd.SetErr(&stderr)
			root.Cmd.SetArgs([]string{"list", "--format", format})

			if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if !strings.Contains(stderr.String(), "Source:") {
				t.Fatalf("--format %s dropped the provenance footer from stderr\n"+
					"stderr: %q", format, stderr.String())
			}
		})
	}
}

// TestFlatFormatsAcrossLeaves guards the other leaves. list was the
// reported case, but show, query --explain, providers, and refresh
// --dry-run all render through the same path — and their payload structs
// carry no `table:""` tags by default, so they emit nothing in the flat
// formats even once the envelope wrapper is out of the way.
//
// Each case names a leaf, its args, and a substring its output must
// contain, so a formatter that emits only a header row still fails.
func TestFlatFormatsAcrossLeaves(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"show", []string{"show", "anthropic", "claude-opus-4-5"}, "claude-opus-4-5"},
		{"query", []string{"query", "claude"}, "claude"},
		{"query-explain", []string{"query", "family:claude", "--explain"}, "family"},
		{"providers", []string{"providers"}, "anthropic"},
		{"refresh-dry-run", []string{"refresh", "--dry-run"}, "would_"},
	}

	for _, tc := range cases {
		for _, format := range []string{"csv", "text", "human"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				primeCache(t, fixturePayload(t))

				root := testRoot(t)
				root.Cmd.AddCommand(
					ListCmd(root), ShowCmd(root), QueryCmd(root),
					ProvidersCmd(root), RefreshCmd(root),
				)
				var stdout, stderr bytes.Buffer
				root.Cmd.SetOut(&stdout)
				root.Cmd.SetErr(&stderr)
				root.Cmd.SetArgs(append(append([]string{}, tc.args...), "--format", format))

				if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
					t.Fatalf("execute %v --format %s: %v\nstderr: %s",
						tc.args, format, err, stderr.String())
				}
				if stdout.Len() == 0 {
					t.Fatalf("%v --format %s emitted 0 bytes on stdout",
						tc.args, format)
				}
				if !strings.Contains(stdout.String(), tc.want) {
					t.Fatalf("%v --format %s output missing %q\ngot: %.400s",
						tc.args, format, tc.want, stdout.String())
				}
			})
		}
	}
}

// TestCSVFormatIsWellFormed pins the CSV shape specifically: a header row
// derived from the `table:""` tags plus one line per model. CSV silently
// degrading to an empty document is indistinguishable from "no results"
// for a consumer piping into a spreadsheet or a dataframe.
func TestCSVFormatIsWellFormed(t *testing.T) {
	primeCache(t, fixturePayload(t))

	root := testRoot(t)
	root.Cmd.AddCommand(ListCmd(root))
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"list", "--format", "csv"})

	if err := root.Cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, stderr.String())
	}

	// Parse with encoding/csv rather than splitting on commas: modality
	// lists are themselves comma-joined and legitimately quoted, so a
	// naive split would misread a well-formed row as ragged.
	records, err := csv.NewReader(bytes.NewReader(stdout.Bytes())).ReadAll()
	if err != nil {
		t.Fatalf("csv output is not parseable: %v\nraw: %.400s", err, stdout.String())
	}
	if len(records) < 2 {
		t.Fatalf("csv output has %d record(s); want header + at least one row\nraw: %q",
			len(records), stdout.String())
	}

	header := records[0]
	for _, col := range []string{"Provider", "Model ID", "Name"} {
		if !slices.Contains(header, col) {
			t.Fatalf("csv header missing %q column\nheader: %v", col, header)
		}
	}
	// csv.Reader already enforces a uniform field count, so reaching here
	// means every row matched the header width. Assert the quoted-field
	// case specifically: a multi-modality value must survive as one field.
	providerIdx := slices.Index(header, "Provider")
	for i, rec := range records[1:] {
		if len(rec) != len(header) {
			t.Fatalf("csv row %d has %d fields, header has %d\nrow: %v",
				i+1, len(rec), len(header), rec)
		}
		if rec[providerIdx] == "" {
			t.Fatalf("csv row %d has an empty Provider field\nrow: %v", i+1, rec)
		}
	}
}
