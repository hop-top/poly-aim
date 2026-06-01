// Command aim is the AI model registry CLI. It queries models.dev (or a
// configured upstream source) and emits structured, agent-friendly output
// over a kit-conformant command surface.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"hop.top/aim/internal/apiversion"
	"hop.top/aim/internal/cmd"
	"hop.top/aim/internal/status"
	speccli "hop.top/kit/go/ai/toolspec/cli"
	"hop.top/kit/go/console/cli"
)

const aimVersion = "0.1.0"

func main() {
	root := cli.New(cli.Config{
		Name:    "aim",
		Version: aimVersion,
		Short:   "AI model registry — query models.dev",
		Accent:  "#7DFFB3", // mint green
	}, cli.WithStatus(cli.StatusConfig{
		ExtraEnvKeys: []string{"AIM_*", "XDG_*", "NO_COLOR", "GOWORK"},
	}))
	root.Cmd.Long = "aim is a fast, machine-readable index of the AI model " +
		"landscape. It mirrors the models.dev registry locally and exposes " +
		"a 12-factor AI-CLI surface — structured output, capability " +
		"introspection (aim spec), runtime status (aim status), and " +
		"explicit per-command side-effect / idempotency contracts for " +
		"agent dispatch."
	status.Register(root, aimVersion)
	root.Cmd.AddCommand(
		cmd.ListCmd(root),
		cmd.ShowCmd(root),
		cmd.ProvidersCmd(root),
		cmd.RefreshCmd(root),
		cmd.QueryCmd(root),
	)
	if err := speccli.RegisterSpecCommand(root, apiversion.Current); err != nil {
		fmt.Fprintln(os.Stderr, "aim: register spec command:", err)
		os.Exit(1)
	}

	// Factor 12 — Evolution Guarantees: gate the kit-registered
	// --api-version flag through apiversion.Negotiate so unsupported
	// values fail fast with a structured envelope rendered by kit's
	// WrapRunE middleware (envelope is INVALID_API_VERSION-coded under
	// the AIM_INVALID_FLAG umbrella). We deliberately do NOT set
	// cli.SetMinAPIVersion — kit's built-in min-version path exits
	// with a bare stderr message via os.Exit(2), bypassing the
	// envelope renderer and yielding inconsistent output for agents.
	installAPIVersionGuard(root)

	if err := root.Execute(context.Background()); err != nil {
		os.Exit(1)
	}
}

// installAPIVersionGuard walks the root subtree and wraps every leaf
// RunE so [apiversion.Negotiate] runs before adopter code. Returning
// the *output.Error from RunE (not PersistentPreRunE) routes the
// failure through kit's WrapRunE middleware, which renders the
// envelope to stderr in the active --format. PersistentPreRunE
// failures bypass that middleware and surface as fang-styled error
// boxes instead of structured envelopes.
//
// The guard runs on every adopter and kit-shipped leaf, so an
// unsupported --api-version errors uniformly regardless of which
// command was invoked. We walk the tree at install time rather than
// patching each leaf in internal/cmd because that package owns its
// own RunE composition and must not pick up Factor 12 coupling.
func installAPIVersionGuard(root *cli.Root) {
	walkCommands(root.Cmd, func(c *cobra.Command) {
		if c.RunE == nil {
			return
		}
		inner := c.RunE
		c.RunE = func(cmd *cobra.Command, args []string) error {
			var requested string
			if root.Viper != nil {
				requested = root.Viper.GetString("api-version")
			}
			if _, envErr := apiversion.Negotiate(requested); envErr != nil {
				return envErr
			}
			return inner(cmd, args)
		}
	})
}

// walkCommands invokes fn for cmd and every transitively-attached
// child. Used to install per-leaf RunE wrappers without depending on
// kit's private walk helpers.
func walkCommands(cmd *cobra.Command, fn func(*cobra.Command)) {
	if cmd == nil {
		return
	}
	fn(cmd)
	for _, child := range cmd.Commands() {
		walkCommands(child, fn)
	}
}
