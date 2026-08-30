package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"hop.top/aim"
	"hop.top/aim/internal/errs"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// modelDetailRow is the flat projection of a single model, used only by
// the tag-driven formats (csv, text, human).
//
// json and yaml keep emitting the full nested [aim.Model] — that is the
// declared output schema and agents depend on its shape. But aim.Model is
// a library type describing the models.dev wire format, and giving it
// `table:""` tags would push CLI presentation concerns into the public
// API. The flat formats cannot render nested structs anyway, so they get
// this denormalised view instead.
type modelDetailRow struct {
	Provider    string `table:"Provider"     json:"provider"     yaml:"provider"`
	ID          string `table:"Model ID"     json:"id"           yaml:"id"`
	Name        string `table:"Name"         json:"name"         yaml:"name"`
	Family      string `table:"Family"       json:"family"       yaml:"family"`
	Input       string `table:"Input"        json:"input"        yaml:"input"`
	Output      string `table:"Output"       json:"output"       yaml:"output"`
	ToolCall    bool   `table:"Tool Call"    json:"tool_call"    yaml:"tool_call"`
	Reasoning   bool   `table:"Reasoning"    json:"reasoning"    yaml:"reasoning"`
	OpenWeights bool   `table:"Open Weights" json:"open_weights" yaml:"open_weights"`
	Attachment  bool   `table:"Attachment"   json:"attachment"   yaml:"attachment"`
	Context     int    `table:"Context"      json:"context"      yaml:"context"`
	InputLimit  int    `table:"Input Limit"  json:"input_limit"  yaml:"input_limit"`
	OutputLimit int    `table:"Output Limit" json:"output_limit" yaml:"output_limit"`
	ReleaseDate string `table:"Released"     json:"release_date" yaml:"release_date"`
	Knowledge   string `table:"Knowledge"    json:"knowledge"    yaml:"knowledge"`
}

// toDetailRow flattens m for the tag-driven formats.
func toDetailRow(m aim.Model) modelDetailRow {
	return modelDetailRow{
		Provider:    m.Provider,
		ID:          m.ID,
		Name:        m.Name,
		Family:      m.Family,
		Input:       strings.Join(m.Modalities.Input, ","),
		Output:      strings.Join(m.Modalities.Output, ","),
		ToolCall:    m.ToolCall,
		Reasoning:   m.Reasoning,
		OpenWeights: m.OpenWeights,
		Attachment:  m.Attachment,
		Context:     m.Limit.Context,
		InputLimit:  m.Limit.Input,
		OutputLimit: m.Limit.Output,
		ReleaseDate: m.ReleaseDate,
		Knowledge:   m.Knowledge,
	}
}

// ShowCmd returns the `show` subcommand.
func ShowCmd(root *cli.Root) *cobra.Command {
	var (
		formatFlag   string
		providerFlag string
		modelFlag    string
	)

	cmd := &cobra.Command{
		Use:   "show [provider] [model]",
		Short: "Show full details for a model",
		Long: `Show the full capability record for a single model.

Identify the model either by two positional arguments (provider then
model) or by the --provider and --model flag aliases. Both forms accept
the same values. When both forms are present, the positional values
win and a warning is emitted; agents that build invocations
programmatically should prefer the flag form for clarity.

Both IDs come from "aim providers" and "aim list" output. Reads from
the local cache; run "aim refresh" first if the model is missing.

Examples:
  aim show openai gpt-4o
  aim show --provider openai --model gpt-4o
  aim show anthropic claude-sonnet-4-5 --format json
`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			var (
				providerID string
				modelID    string
				warnings   []string
			)
			switch len(args) {
			case 2:
				providerID, modelID = args[0], args[1]
				if providerFlag != "" || modelFlag != "" {
					warnings = append(warnings,
						"show: positional and flag forms both supplied; "+
							"positional values win — prefer the "+
							"--provider/--model flag form for clarity")
				}
			case 0:
				if providerFlag == "" || modelFlag == "" {
					return errs.InvalidFlag(
						"provider/model",
						"",
						"supply either two positional args or both --provider and --model")
				}
				providerID, modelID = providerFlag, modelFlag
			default:
				return errs.InvalidFlag(
					"args",
					fmt.Sprintf("%d", len(args)),
					"show takes 0 (with flags) or 2 (positional) arguments")
			}

			format := formatFlag
			if format == "" {
				format = root.Viper.GetString("format")
			}
			if format == "table" && !cmd.Flags().Changed("format") {
				format = defaultFormat()
			}

			reg := aim.NewRegistry()
			// First check whether the provider exists at all so the
			// envelope can distinguish provider-not-found from
			// model-not-found.
			providers, err := reg.Providers(ctx)
			if err != nil {
				return errs.FromRefreshError(aim.DefaultSourceURL, err)
			}
			providerOK := false
			for _, p := range providers {
				if p.ID == providerID {
					providerOK = true
					break
				}
			}
			if !providerOK {
				return errs.NotFound("provider", providerID)
			}
			m, ok, err := reg.Get(ctx, providerID, modelID)
			if err != nil {
				return errs.FromRefreshError(aim.DefaultSourceURL, err)
			}
			if !ok {
				return errs.NotFound("model", providerID+"/"+modelID)
			}

			meta := provenanceFromCache(reg.Cache(), reg.SourceURL())
			if format == output.Table {
				if err := renderModelDetail(cmd, m, meta); err != nil {
					return err
				}
				for _, w := range warnings {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
				}
				return nil
			}
			// Flat formats cannot express aim.Model's nested shape, so
			// they render the denormalised projection instead. json and
			// yaml keep the full record — that is the declared schema.
			var payload any = m
			if flatFormats[format] {
				payload = toDetailRow(m)
			}
			return renderWithWarnings(
				cmd.OutOrStdout(), cmd.ErrOrStderr(),
				format, payload, meta, warnings,
			)
		},
	}

	cmd.Flags().StringVar(&formatFlag, "format", "", "Output format: table, json, yaml")
	cmd.Flags().StringVar(&providerFlag, "provider", "", "Provider ID (alias for first positional)")
	cmd.Flags().StringVar(&modelFlag, "model", "", "Model ID (alias for second positional)")

	cli.SetSideEffect(cmd, cli.SideEffectRead)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	cli.SetTopLevelVerb(cmd)
	setExitCodes(cmd, exitCodesLookup)
	_ = cli.SetOutputSchema(cmd, cli.OutputSchema{
		Type:    &aim.Model{},
		Version: SchemaVersion,
	})
	_ = cli.SetExamples(cmd, []cli.Example{
		{Title: "OpenAI flagship (positional)", Command: "aim show openai gpt-4o"},
		{Title: "OpenAI flagship (flag form)", Command: "aim show --provider openai --model gpt-4o"},
		{Title: "JSON for agents", Command: "aim show anthropic claude-sonnet-4-5 --format json"},
	})
	_ = cli.SetNextSteps(cmd, []cli.NextStep{
		{
			When:    "model not found in cache",
			Suggest: "aim refresh",
			Reason:  "Local cache may be stale; refresh and retry",
		},
		{
			When:    "browse the same provider's catalog",
			Suggest: "aim list --provider <provider>",
			Reason:  "Confirm the correct provider+model pair",
		},
		{
			When:    "compare alternatives in the same family",
			Suggest: "aim query \"family:<family>\"",
			Reason:  "Surface sibling models with shared capabilities",
		},
	})

	registerHints(root, "show",
		output.Hint{Message: "Sibling models: `aim list --provider <provider>`."},
		output.Hint{Message: "Compare family: `aim query \"family:<family>\"`."},
		output.Hint{Message: "Not found? `aim refresh` then retry (cache may be stale)."},
	)
	installHintEmitter(root, cmd, "show")
	return cmd
}

// renderModelDetail writes a human-readable model detail block. It
// mirrors kit's WithProvenance behavior for table format: the payload
// stays on stdout, a single provenance footer is written to stderr so
// agents that capture stderr still see source/fetched/method without
// polluting the human surface.
func renderModelDetail(cmd *cobra.Command, m aim.Model, meta output.Metadata) error {
	w := cmd.OutOrStdout()
	line := func(k, v string) { _, _ = fmt.Fprintf(w, "  %-16s %s\n", k+":", v) }
	boolStr := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}

	_, _ = fmt.Fprintf(w, "%s / %s\n", m.Provider, m.ID)
	if m.Name != "" {
		line("Name", m.Name)
	}
	if m.Family != "" {
		line("Family", m.Family)
	}
	line("Input", strings.Join(m.Modalities.Input, ", "))
	line("Output", strings.Join(m.Modalities.Output, ", "))
	line("Tool Call", boolStr(m.ToolCall))
	line("Reasoning", boolStr(m.Reasoning))
	line("Open Weights", boolStr(m.OpenWeights))
	line("Attachment", boolStr(m.Attachment))
	if m.Limit.Context > 0 {
		line("Context", fmt.Sprintf("%d tokens", m.Limit.Context))
	}
	if m.Limit.Input > 0 {
		line("Input Limit", fmt.Sprintf("%d tokens", m.Limit.Input))
	}
	if m.Limit.Output > 0 {
		line("Output Limit", fmt.Sprintf("%d tokens", m.Limit.Output))
	}
	if m.ReleaseDate != "" {
		line("Released", m.ReleaseDate)
	}
	if m.Knowledge != "" {
		line("Knowledge", m.Knowledge)
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Source: %s (fetched %s, method=%s)\n",
		meta.Source, meta.FetchedAt.Format(time.RFC3339), meta.Method)
	return nil
}
