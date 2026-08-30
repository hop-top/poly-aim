// Command aim is the AI model registry CLI. It queries models.dev (or a
// configured upstream source) and emits structured, agent-friendly output
// over a kit-conformant command surface.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"hop.top/aim/internal/apiversion"
	"hop.top/aim/internal/cmd"
	"hop.top/aim/internal/status"
	speccli "hop.top/kit/go/ai/toolspec/cli"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// aimVersion is the version reported by `aim --version` and stamped into
// the spec manifest.
//
// The trailing annotation is a release-please directive: the release PR
// rewrites this literal in lock-step with .github/.release-please-manifest.json,
// so the binary can never again drift from the released version. There is
// no ldflags injection point to use instead — the Go entry in publish.yml
// is mirror-only and builds no binary.
const aimVersion = "0.1.0-alpha.4" // x-release-please-version

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

	// After every command is mounted: unmatched positionals on group
	// nodes become usage errors instead of cobra's help-and-exit-0.
	hardenCommandGroups(root.Cmd)

	if err := root.Execute(context.Background()); err != nil {
		os.Exit(exitCode(err))
	}
}

// exitCode resolves the process exit code for an error returned by
// Execute. Structured envelopes (and wrappers exposing AsCLIError)
// carry their own classified code from the shared taxonomy — 0 success,
// 1 general, 2 usage, 3 not-found, 4 conflict, 5 permission,
// 6 transient/retryable, 64 rate-limited. Anything else is a general
// failure (1).
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ce interface{ AsCLIError() *output.Error }
	if errors.As(err, &ce) {
		if e := ce.AsCLIError(); e != nil && e.ExitCode != 0 {
			return e.ExitCode
		}
	}
	var oe *output.Error
	if errors.As(err, &oe) && oe.ExitCode != 0 {
		return oe.ExitCode
	}
	return 1
}

// rejectUnknownSubcommand turns unmatched positionals on a group command
// into structured usage errors (exit 2) instead of cobra's silent
// help-and-exit-0, which reads to an agent as a successful invocation.
func rejectUnknownSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	fix := "run `" + cmd.CommandPath() + " --help` to see available commands"
	return &output.Error{
		Code: output.CodeUsage,
		Message: fmt.Sprintf("unknown command %q for %q — %s",
			args[0], cmd.CommandPath(), fix),
		SuggestedFix: fix,
		ExitCode:     2,
	}
}

// runGroupHelp keeps a bare group invocation on the help path.
func runGroupHelp(cmd *cobra.Command, _ []string) error { return cmd.Help() }

// hardenCommandGroups walks the mounted tree and, for every node with
// subcommands, keeps the bare invocation on the help path while
// rejecting unmatched positionals as usage errors.
//
// Cobra only arg-validates runnable commands, so a non-runnable group
// silently discards `aim frobnicate` and exits 0. Groups therefore gain
// a help-rendering RunE alongside the Args validator.
//
// Must be called after every AddCommand: the walk only sees what is
// already mounted. Leaves are left untouched — they own their own Args
// contracts (show and query both take positionals).
func hardenCommandGroups(root *cobra.Command) {
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, c := range cmd.Commands() {
			walk(c)
		}
		if !cmd.HasSubCommands() {
			return
		}
		if !cmd.Runnable() {
			cmd.RunE = runGroupHelp
		}
		cmd.Args = rejectUnknownSubcommand
	}
	walk(root)
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
