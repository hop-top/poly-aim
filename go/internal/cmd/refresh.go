package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"hop.top/aim"
	"hop.top/aim/internal/errs"
	"hop.top/kit/go/console/cli"
	kitlog "hop.top/kit/go/console/log"
	"hop.top/kit/go/console/output"
	"hop.top/kit/go/core/xdg"
)

// RefreshCmd returns the `refresh` subcommand.
func RefreshCmd(root *cli.Root) *cobra.Command {
	var (
		forceFlag  bool
		formatFlag string
	)

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the local model registry cache",
		Long: `Refresh the local model-registry cache from the upstream source
(models.dev by default).

Writes only to the XDG cache directory; no shared state is touched.
Naturally idempotent: last-write-wins atomic rename guarded by a
lockfile, so concurrent invocations are safe. Pass --force to ignore
the active TTL and re-fetch even when the cache is fresh.

Examples:
  aim refresh
  aim refresh --force
`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			log := kitlog.New(root.Viper)

			format := formatFlag
			if format == "" {
				format = root.Viper.GetString("format")
			}
			if format == "table" && !cmd.Flags().Changed("format") {
				format = defaultFormat()
			}

			// Dry-run short-circuits before any fetch / write. Inspect
			// cache meta only; no network, no disk mutation.
			if cli.IsDryRun(cmd) {
				return runRefreshDryRun(cmd, format, forceFlag)
			}

			log.Info("refreshing registry cache...")

			var opts []aim.Option
			if forceFlag {
				opts = append(opts, aim.WithCacheOpts(aim.WithTTL(0)))
			}

			reg := aim.NewRegistry(opts...)
			if err := reg.Refresh(ctx); err != nil {
				return errs.FromRefreshError(aim.DefaultSourceURL, err)
			}

			meta := provenanceForRefresh(reg.SourceURL())
			payload := refreshStatus{
				Refreshed:   true,
				CachedUntil: meta.FetchedAt.Add(cacheLifetimeFromMeta(reg.Cache())).Format(time.RFC3339),
				Source:      meta.Source,
			}
			if format == output.Table {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Registry cache refreshed.")
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Source: %s (fetched %s, method=%s)\n",
					meta.Source, meta.FetchedAt.Format(time.RFC3339), meta.Method)
				return nil
			}
			return renderEnvelope(cmd.OutOrStdout(), cmd.ErrOrStderr(), format, payload, meta)
		},
	}

	cmd.Flags().BoolVar(&forceFlag, "force", false, "Ignore TTL and force a full re-fetch")
	cmd.Flags().StringVar(&formatFlag, "format", "", "Output format: table, json, yaml")

	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	cli.SetTopLevelVerb(cmd)
	setExitCodes(cmd, exitCodesRead)
	_ = cli.SetOutputSchema(cmd, cli.OutputSchema{
		Type:    &refreshStatus{},
		Version: SchemaVersion,
	})
	_ = cli.SetExamples(cmd, []cli.Example{
		{Title: "Normal refresh", Command: "aim refresh"},
		{Title: "Force re-fetch", Command: "aim refresh --force"},
	})
	_ = cli.SetNextSteps(cmd, []cli.NextStep{
		{
			When:    "cache populated — verify state",
			Suggest: "aim status --format json",
			Reason:  "Confirm source URL, fetched_at, TTL after a successful refresh",
		},
		{
			When:    "cache populated — browse the catalog",
			Suggest: "aim list",
			Reason:  "List models from the freshly populated cache",
		},
		{
			When:    "TTL still active — force re-fetch",
			Suggest: "aim refresh --force",
			Reason:  "Bypass the cache lifetime when upstream changed under TTL",
		},
		{
			When:    "refresh fails — diagnose the upstream",
			Suggest: "aim status --format json | jq '.sections[] | select(.title == \"source\")'",
			Reason:  "Inspect source URL, last fetch, and cache paths",
		},
	})

	registerHints(root, "refresh",
		output.Hint{Message: "Verify state: `aim status --format json`."},
		output.Hint{Message: "Browse the cache: `aim list`."},
		output.Hint{Message: "Force re-fetch (ignores TTL): `aim refresh --force`."},
	)
	installHintEmitter(root, cmd, "refresh")
	_ = cli.SetDryRunRationale(cmd,
		"--dry-run would print the upstream URL, ETag, and target cache paths without writing")
	return cmd
}

// cacheLifetimeFromMeta returns the active cache TTL recorded in
// meta.json after a successful refresh. Falls back to [aim.DefaultCacheTTL]
// when the meta is unreadable (covers first-run / corrupt paths).
func cacheLifetimeFromMeta(c *aim.Cache) time.Duration {
	if c == nil {
		return aim.DefaultCacheTTL
	}
	if m := c.Meta(); m.Present && m.TTL > 0 {
		return m.TTL
	}
	return aim.DefaultCacheTTL
}

// runRefreshDryRun renders a [RefreshPreview] describing what `refresh`
// WOULD do without applying any changes. It MUST NOT touch the
// network, MUST NOT write to disk, and MUST exit 0.
//
// Decision rules:
//   - force=true ⇒ would_refresh, reason=force (TTL ignored).
//   - meta absent ⇒ would_refresh, reason=no_prior_fetch.
//   - meta present + within TTL ⇒ would_skip, reason=ttl_remaining.
//   - meta present + TTL expired ⇒ would_refresh, reason=ttl_expired.
func runRefreshDryRun(cmd *cobra.Command, format output.Format, force bool) error {
	// We build the registry without any cache-mutating options so the
	// Cache() accessor is purely read-only.
	reg := aim.NewRegistry()
	c := reg.Cache()
	meta := c.Meta()

	dir := refreshCacheDir(c)
	preview := RefreshPreview{
		WouldFetchURL: reg.SourceURL(),
		WouldWritePaths: []string{
			filepath.Join(dir, "models-dev.json"),
			filepath.Join(dir, "meta.json"),
		},
	}
	if meta.Present {
		preview.CurrentETag = meta.ETag
		if !meta.LastFetch.IsZero() {
			preview.CurrentLastFetch = meta.LastFetch.Format(time.RFC3339)
			preview.CurrentAge = formatDuration(time.Since(meta.LastFetch))
		}
	}

	ttl := aim.DefaultCacheTTL
	if meta.Present && meta.TTL > 0 {
		ttl = meta.TTL
	}

	switch {
	case force:
		preview.Status = "would_refresh"
		preview.Reason = "force"
		preview.WouldSkipDueToTTL = false
	case !meta.Present || meta.LastFetch.IsZero():
		preview.Status = "would_refresh"
		preview.Reason = "no_prior_fetch"
		preview.WouldSkipDueToTTL = false
	default:
		age := time.Since(meta.LastFetch)
		if age < ttl {
			preview.Status = "would_skip"
			preview.Reason = "ttl_remaining"
			preview.WouldSkipDueToTTL = true
			preview.TTLRemaining = formatDuration(ttl - age)
		} else {
			preview.Status = "would_refresh"
			preview.Reason = "ttl_expired"
			preview.WouldSkipDueToTTL = false
		}
	}

	prov := provenanceFromCache(c, reg.SourceURL())

	if format == output.Table {
		w := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(w, "dry-run: %s (%s)\n", preview.Status, preview.Reason)
		_, _ = fmt.Fprintf(w, "  would_fetch_url: %s\n", preview.WouldFetchURL)
		for _, p := range preview.WouldWritePaths {
			_, _ = fmt.Fprintf(w, "  would_write:     %s\n", p)
		}
		if preview.CurrentLastFetch != "" {
			_, _ = fmt.Fprintf(w, "  last_fetch:      %s (age %s)\n",
				preview.CurrentLastFetch, preview.CurrentAge)
		}
		if preview.TTLRemaining != "" {
			_, _ = fmt.Fprintf(w, "  ttl_remaining:   %s\n", preview.TTLRemaining)
		}
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Source: %s (fetched %s, method=%s)\n",
			prov.Source, prov.FetchedAt.Format(time.RFC3339), prov.Method)
		return nil
	}
	return renderEnvelope(cmd.OutOrStdout(), cmd.ErrOrStderr(), format, preview, prov)
}

// refreshCacheDir extracts the cache directory from an [*aim.Cache]
// without forcing a fetch. When the cache's Dir override is empty we
// fall back to the XDG default that aim.Cache itself would resolve at
// fetch time. Mirrors aim/cache.go::dir; kept in lockstep so the
// dry-run preview reports the real on-disk target.
func refreshCacheDir(c *aim.Cache) string {
	if c != nil && c.Dir != "" {
		return c.Dir
	}
	base, err := xdg.CacheDir("hop")
	if err != nil {
		// Diagnostic only — preview still renders. Cache writes will
		// surface the same error on the real run.
		return "<xdg-cache-dir>/hop/aim"
	}
	return filepath.Join(base, "aim")
}
