package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"hop.top/aim"
	"hop.top/aim/internal/errs"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// modelRow is the table projection of a Model for list/query output.
type modelRow struct {
	Provider    string `table:"Provider"   json:"provider"    yaml:"provider"`
	ID          string `table:"Model ID"   json:"id"          yaml:"id"`
	Name        string `table:"Name"       json:"name"        yaml:"name"`
	Input       string `table:"Input"      json:"input"       yaml:"input"`
	Output      string `table:"Output"     json:"output"      yaml:"output"`
	ToolCall    bool   `table:"ToolCall"   json:"tool_call"   yaml:"tool_call"`
	Reasoning   bool   `table:"Reasoning"  json:"reasoning"   yaml:"reasoning"`
}

func toRow(m aim.Model) modelRow {
	return modelRow{
		Provider:  m.Provider,
		ID:        m.ID,
		Name:      m.Name,
		Input:     strings.Join(m.Modalities.Input, ","),
		Output:    strings.Join(m.Modalities.Output, ","),
		ToolCall:  m.ToolCall,
		Reasoning: m.Reasoning,
	}
}

func toRows(models []aim.Model) []modelRow {
	rows := make([]modelRow, len(models))
	for i, m := range models {
		rows[i] = toRow(m)
	}
	return rows
}

// ListCmd returns the `list` subcommand.
func ListCmd(root *cli.Root) *cobra.Command {
	var (
		inputFlag       []string
		outputFlag      []string
		providerFlag    string
		familyFlag      string
		toolCallFlag    bool
		reasoningFlag   bool
		openWeightsFlag bool
		hasToolCall     bool
		hasReasoning    bool
		hasOpenWeights  bool
		formatFlag      string
	)

	cmd := &cobra.Command{
		Use:   "list [query]",
		Short: "List models, optionally filtered by flags",
		Long: `List models from the AI model registry.

Prefer flag-built filters (--family, --provider, --tool-call, …).
Reads from the local cache; run "aim refresh" first if the cache is
empty or stale.

DEPRECATED: passing a free-text query as a positional argument still
works but emits a warning. Use "aim query <expr>" for the DSL syntax,
or the equivalent flag form (e.g. --family).

Examples:
  aim list                                # all models, table form
  aim list --provider openai --tool-call  # OpenAI models with tool-call
  aim query "gpt"                         # free-text query — preferred form
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			reg := aim.NewRegistry()

			format := formatFlag
			if format == "" {
				format = root.Viper.GetString("format")
			}
			if format == "table" && !cmd.Flags().Changed("format") {
				format = defaultFormat()
			}

			var models []aim.Model
			var err error
			var warnings []string

			if len(args) == 1 {
				// free-text query — DEPRECATED. Still functions so we
				// don't break humans mid-flight, but the warning
				// surfaces in envelope.warnings for JSON/YAML and on
				// stderr for table mode.
				warnings = append(warnings,
					"list: positional arguments are deprecated; "+
						"use 'aim query <expr>' for DSL syntax or "+
						"'--family foo' for flag filters")
				models, err = reg.Query(ctx, args[0])
				if err != nil {
					// ParseQuery errors emerge here as well as deeper
					// load errors. Detect parser errors by the package
					// prefix the aim lib stamps in.
					if strings.HasPrefix(err.Error(), "aim: ") &&
						(strings.Contains(err.Error(), "tag") ||
							strings.Contains(err.Error(), "query") ||
							strings.Contains(err.Error(), "colon")) {
						return errs.InvalidQuery(args[0], err)
					}
					return errs.FromRefreshError(aim.DefaultSourceURL, err)
				}
			} else {
				// flags-based filter
				f := aim.Filter{
					Input:    inputFlag,
					Output:   outputFlag,
					Provider: providerFlag,
					Family:   familyFlag,
				}
				if hasToolCall {
					f.ToolCall = boolPtr(toolCallFlag)
				}
				if hasReasoning {
					f.Reasoning = boolPtr(reasoningFlag)
				}
				if hasOpenWeights {
					f.OpenWeights = boolPtr(openWeightsFlag)
				}
				models, err = reg.Models(ctx, f)
				if err != nil {
					return errs.FromRefreshError(aim.DefaultSourceURL, err)
				}
			}

			rows := toRows(models)
			meta := provenanceFromCache(reg.Cache(), reg.SourceURL())
			return renderWithWarnings(
				cmd.OutOrStdout(), cmd.ErrOrStderr(),
				format, rows, meta, warnings,
			)
		},
	}

	cmd.Flags().StringSliceVar(&inputFlag, "input", nil, "Filter by input modality (e.g. image,text)")
	cmd.Flags().StringSliceVar(&outputFlag, "output", nil, "Filter by output modality")
	cmd.Flags().StringVar(&providerFlag, "provider", "", "Filter by provider ID")
	cmd.Flags().StringVar(&familyFlag, "family", "", "Filter by model family")
	cmd.Flags().BoolVar(&toolCallFlag, "tool-call", false, "Require tool-call support")
	cmd.Flags().BoolVar(&reasoningFlag, "reasoning", false, "Require reasoning/CoT support")
	cmd.Flags().BoolVar(&openWeightsFlag, "open-weights", false, "Require open-weights availability")
	cmd.Flags().StringVar(&formatFlag, "format", "", "Output format: table, json, yaml")

	// Track whether tristate flags were explicitly set.
	cmd.PreRunE = func(c *cobra.Command, _ []string) error {
		hasToolCall = c.Flags().Changed("tool-call")
		hasReasoning = c.Flags().Changed("reasoning")
		hasOpenWeights = c.Flags().Changed("open-weights")
		return nil
	}

	cli.SetSideEffect(cmd, cli.SideEffectRead)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	cli.SetTopLevelVerb(cmd)
	_ = cli.SetOutputSchema(cmd, cli.OutputSchema{
		Type:    &[]modelRow{},
		Version: SchemaVersion,
	})
	_ = cli.SetExamples(cmd, []cli.Example{
		{Title: "Image-input models", Command: "aim list --input image"},
		{Title: "Tool-call OpenAI", Command: "aim list --provider openai --tool-call"},
		{Title: "Free-text — use query", Command: "aim query \"gpt\" --format json"},
	})
	_ = cli.SetNextSteps(cmd, []cli.NextStep{
		{
			When:    "results show a candidate model",
			Suggest: "aim show <provider> <model-id>",
			Reason:  "Pull full capability detail for the chosen model",
		},
		{
			When:    "no results returned — refresh or widen filters",
			Suggest: "aim refresh",
			Reason:  "Local cache may be empty or stale",
		},
		{
			When:    "want DSL-grade filtering",
			Suggest: "aim query <expr>",
			Reason:  "Structured tag DSL is more expressive than flag combos",
		},
	})

	registerHints(root, "list",
		output.Hint{Message: "Refine: `aim query \"<expr>\"` for DSL filtering."},
		output.Hint{Message: "Pull detail: `aim show <provider> <model-id>`."},
		output.Hint{Message: "Stale cache? `aim refresh` then re-run."},
		output.Hint{Message: "Machine output: re-run with `--format json` for the envelope."},
	)
	installHintEmitter(root, cmd, "list")
	return cmd
}
