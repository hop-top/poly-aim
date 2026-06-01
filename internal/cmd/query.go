package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"hop.top/aim"
	"hop.top/aim/internal/errs"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// QueryCmd returns the `query` subcommand.
func QueryCmd(root *cli.Root) *cobra.Command {
	var (
		formatFlag  string
		explainFlag bool
	)

	cmd := &cobra.Command{
		Use:   "query <query>",
		Short: "Query models using the aim query syntax",
		Long: `Query models using the structured aim query syntax.

Supports key:value tags ANDed with free-text tokens. Tags are stable
across language SDKs (Go, TS, Python). Pure read; consults the local
cache only.

Tag grammar:
  provider:<id>            exact provider match
  in:<modality,...>        required input modalities (comma list)
  out:<modality,...>       required output modalities (comma list)
  tool_call:true|false     require tool-calling support
  reasoning:true|false     require reasoning/CoT support
  open_weights:true|false  require open-weights availability
  <free-text>              substring match on name/id

Pass --explain to dry-parse the expression and return the parsed AST
instead of running the query. Useful for agents that want to validate
DSL shape before executing.

Examples:
  aim query "provider:openai tool_call:true"
  aim query "in:image,text reasoning:true" --format json
  aim query --explain "provider:openai tool_call:true" --format json
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			format := formatFlag
			if format == "" {
				format = root.Viper.GetString("format")
			}
			if format == "table" && !cmd.Flags().Changed("format") {
				format = defaultFormat()
			}

			// --explain short-circuits the registry path. We parse the
			// expression and emit the AST. No cache read, no network.
			if explainFlag {
				exp, perr := aim.ExplainQuery(args[0])
				if perr != nil {
					return errs.InvalidQuery(args[0], perr)
				}
				payload := QueryExplain{
					ExprInput: exp.Input,
					Filter:    filterToMap(exp.Filter),
					FreeText:  exp.FreeText,
				}
				// Use the cache provenance so agents see a consistent
				// envelope shape; --explain does not touch the cache,
				// so the meta is built without forcing a load.
				reg := aim.NewRegistry()
				meta := provenanceFromCache(reg.Cache(), reg.SourceURL())
				return output.Render(cmd.OutOrStdout(), format, payload, output.WithProvenance(meta))
			}

			// Pre-parse so we can route parse errors through the
			// INVALID_QUERY envelope before touching the registry.
			if _, perr := aim.ParseQuery(args[0]); perr != nil {
				return errs.InvalidQuery(args[0], perr)
			}

			reg := aim.NewRegistry()
			models, err := reg.Query(ctx, args[0])
			if err != nil {
				// Defensive: anything that slips past the pre-parse
				// gate but still looks like a parser failure routes
				// through INVALID_QUERY; everything else is a load
				// error.
				if strings.HasPrefix(err.Error(), "aim: ") &&
					(strings.Contains(err.Error(), "tag") ||
						strings.Contains(err.Error(), "query") ||
						strings.Contains(err.Error(), "colon")) {
					return errs.InvalidQuery(args[0], err)
				}
				return errs.FromRefreshError(aim.DefaultSourceURL, err)
			}

			rows := toRows(models)
			meta := provenanceFromCache(reg.Cache(), reg.SourceURL())
			return output.Render(cmd.OutOrStdout(), format, rows, output.WithProvenance(meta))
		},
	}

	cmd.Flags().StringVar(&formatFlag, "format", "", "Output format: table, json, yaml")
	cmd.Flags().BoolVar(&explainFlag, "explain", false, "Parse the expression and emit the AST instead of running the query")

	cli.SetSideEffect(cmd, cli.SideEffectRead)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	cli.SetTopLevelVerb(cmd)
	_ = cli.SetOutputSchema(cmd, cli.OutputSchema{
		Type:    &[]modelRow{},
		Version: SchemaVersion,
	})
	_ = cli.SetExamples(cmd, []cli.Example{
		{Title: "OpenAI tool-call", Command: "aim query \"provider:openai tool_call:true\""},
		{Title: "Multimodal reasoning", Command: "aim query \"in:image,text reasoning:true\" --format json"},
	})
	_ = cli.SetNextSteps(cmd, []cli.NextStep{
		{
			When:    "results show a candidate model",
			Suggest: "aim show <provider> <model-id>",
			Reason:  "Pull full capability detail for the chosen model",
		},
		{
			When:    "no results returned — refresh or relax tags",
			Suggest: "aim refresh",
			Reason:  "Local cache may be empty or stale",
		},
		{
			When:    "validate the DSL without running",
			Suggest: "aim query --explain <expr>",
			Reason:  "Dry-run mode echoes the parsed AST without consulting the registry",
		},
		{
			When:    "discover DSL grammar",
			Suggest: "aim spec --format json | jq '.commands[] | select(.path[1] == \"query\") | .examples'",
			Reason:  "Spec manifest lists every canonical example",
		},
	})

	registerHints(root, "query",
		output.Hint{Message: "Dry-run the DSL: `aim query --explain <expr>`."},
		output.Hint{Message: "Pull detail: `aim show <provider> <model-id>`."},
		output.Hint{Message: "Grammar: `aim spec --format json | jq '.commands[] | select(.path[1]==\"query\") | .examples'`."},
	)
	installHintEmitter(root, cmd, "query")
	return cmd
}
