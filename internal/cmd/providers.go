package cmd

import (
	"github.com/spf13/cobra"
	"hop.top/aim"
	"hop.top/aim/internal/errs"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// providerRow is the table projection of a Provider.
type providerRow struct {
	ID     string `table:"ID"     json:"id"     yaml:"id"`
	Name   string `table:"Name"   json:"name"   yaml:"name"`
	Models int    `table:"Models" json:"models" yaml:"models"`
}

// ProvidersCmd returns the `providers` subcommand.
func ProvidersCmd(root *cli.Root) *cobra.Command {
	var formatFlag string

	cmd := &cobra.Command{
		Use:   "providers",
		Short: "List all providers",
		Long: `List every provider in the registry with the number of models each
ships. Pure read; consults the local cache and never touches the
network.

Examples:
  aim providers
  aim providers --format json
`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			format := formatFlag
			if format == "" {
				format = root.Viper.GetString("format")
			}
			if format == "table" && !cmd.Flags().Changed("format") {
				format = defaultFormat()
			}

			reg := aim.NewRegistry()
			providers, err := reg.Providers(ctx)
			if err != nil {
				return errs.FromRefreshError(aim.DefaultSourceURL, err)
			}

			rows := make([]providerRow, len(providers))
			for i, p := range providers {
				rows[i] = providerRow{
					ID:     p.ID,
					Name:   p.Name,
					Models: len(p.Models),
				}
			}
			meta := provenanceFromCache(reg.Cache(), reg.SourceURL())
			return renderEnvelope(cmd.OutOrStdout(), cmd.ErrOrStderr(), format, rows, meta)
		},
	}

	cmd.Flags().StringVar(&formatFlag, "format", "", "Output format: table, json, yaml")

	cli.SetSideEffect(cmd, cli.SideEffectRead)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	cli.SetTopLevelVerb(cmd)
	setExitCodes(cmd, exitCodesRead)
	_ = cli.SetOutputSchema(cmd, cli.OutputSchema{
		Type:    &[]providerRow{},
		Version: SchemaVersion,
	})
	_ = cli.SetExamples(cmd, []cli.Example{
		{Title: "All providers", Command: "aim providers"},
		{Title: "JSON for agents", Command: "aim providers --format json"},
	})
	_ = cli.SetNextSteps(cmd, []cli.NextStep{
		{
			When:    "want models for a provider",
			Suggest: "aim list --provider <id>",
			Reason:  "Scope the list to a single provider",
		},
		{
			When:    "list is empty — prime the cache",
			Suggest: "aim refresh",
			Reason:  "Local cache may be empty",
		},
		{
			When:    "inspect cache state",
			Suggest: "aim status --format json",
			Reason:  "Confirm source, TTL, and last fetch before fanning out",
		},
	})

	registerHints(root, "providers",
		output.Hint{Message: "Drill into a catalog: `aim list --provider <id>`."},
		output.Hint{Message: "Empty list? `aim refresh` to prime the cache."},
		output.Hint{Message: "Cache state: `aim status --format json`."},
	)
	installHintEmitter(root, cmd, "providers")
	return cmd
}
