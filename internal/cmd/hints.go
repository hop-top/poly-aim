package cmd

import (
	"github.com/spf13/cobra"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// registerHints attaches one or more next-step Hints to the named command
// in the Root's shared HintSet. The HintSet is the same registry kit's
// `output.RenderHints` consults; commands look themselves up by their
// kit-canonical name (the cobra Use stem — "list", "show", etc.).
//
// Hint emission is deferred to the PostRunE wrapper installed by
// installHintEmitter: cobra runs PostRunE only after RunE returns nil,
// so failing leaves never produce stale hints.
//
// Centralising the registration call keeps the per-leaf constructor
// surface uniform and gives the spec manifest a single grep target.
func registerHints(root *cli.Root, name string, hints ...output.Hint) {
	if root == nil || root.Hints == nil {
		return
	}
	root.Hints.Register(name, hints...)
}

// installHintEmitter wires a PostRunE on cmd that renders the hints
// registered under name. PostRunE is composed with any pre-existing
// PostRunE on the command, so kit-installed middleware (none today,
// reserved for future use) continues to run.
//
// Emission honours kit's suppression rules end-to-end:
//   - JSON / YAML format → output.RenderHints is a no-op.
//   - Non-TTY stderr (pipes, tests, agents capturing stderr) → no-op.
//   - --no-hints, --quiet, hints.enabled=false, HOP_QUIET_HINTS → no-op.
//
// Tests verify the registration side (`root.Hints.Lookup(name)`) and the
// HintsEnabled flag wiring rather than the rendered output, mirroring
// the kit hint_test.go strategy.
func installHintEmitter(root *cli.Root, cmd *cobra.Command, name string) {
	prev := cmd.PostRunE
	cmd.PostRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		emitHints(root, c, name)
		return nil
	}
}

// emitHints resolves the active --format value (so JSON/YAML correctly
// suppress) and delegates to kit's output.RenderHints. Writes go to
// stderr to keep stdout payload-pure for piped consumers, matching the
// provenance footer convention already in use across aim.
func emitHints(root *cli.Root, cmd *cobra.Command, name string) {
	if root == nil || root.Hints == nil {
		return
	}
	hints := root.Hints.Lookup(name)
	if len(hints) == 0 {
		return
	}
	format := activeHintFormat(root, cmd)
	output.RenderHints(cmd.ErrOrStderr(), hints, format, root.Viper, nil)
}

// activeHintFormat mirrors the format resolution each leaf already
// performs in its RunE: explicit --format on the command wins, else the
// viper-resolved key, else the TTY-aware default. Hint emission only
// needs the eventual Format string so RenderHints can apply the
// JSON/YAML suppression.
func activeHintFormat(root *cli.Root, cmd *cobra.Command) output.Format {
	if pf := cmd.Flags().Lookup("format"); pf != nil && pf.Changed {
		if v := pf.Value.String(); v != "" {
			return v
		}
	}
	if root != nil && root.Viper != nil {
		if v := root.Viper.GetString("format"); v != "" {
			return v
		}
	}
	return defaultFormat()
}
