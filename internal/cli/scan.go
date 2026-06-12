// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/reswaraa/envdoctor/internal/bundle"
	"github.com/reswaraa/envdoctor/internal/config"
	"github.com/reswaraa/envdoctor/internal/engine"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/probes"
	"github.com/reswaraa/envdoctor/internal/recipes"
	"github.com/reswaraa/envdoctor/internal/system"
)

type scanFlags struct {
	jsonOut            bool
	quiet              bool
	bundle             string
	bundleIncludePaths bool
	dryRun             bool
}

func newScanCmd() *cobra.Command {
	var f scanFlags
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan the current repo for environment problems",
		Long: `Scan inspects the current directory for known manifest files,
probes the local system, and reports findings with copy-pasteable repair
commands.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return &exitErr{code: ExitCrashed, err: fmt.Errorf("resolve cwd: %w", err)}
			}
			report, recipeHash, runErr := runScan(cmd.Context(), cwd, f)
			if runErr != nil {
				return runErr
			}
			if err := emitReport(cmd.OutOrStdout(), cmd.ErrOrStderr(), report, recipeHash, f); err != nil {
				return &exitErr{code: ExitCrashed, err: err}
			}
			if code := ExitCodeFor(report); code != ExitOK {
				return &exitErr{code: code}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&f.jsonOut, "json", false, "emit JSON to stdout instead of the pretty TTY view")
	cmd.Flags().BoolVar(&f.quiet, "quiet", false, "(reserved) hide non-failing findings")
	cmd.Flags().StringVar(&f.bundle, "bundle", "", "write a shareable debug bundle to PATH")
	cmd.Flags().BoolVar(&f.bundleIncludePaths, "bundle-include-paths", false, "keep absolute paths verbatim in the bundle (default: redact $HOME → ~ and RepoRoot → basename)")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "no-op for scan; honored by `envdoctor fix`")
	return cmd
}

// runScan collects facts, runs the engine, and returns the populated
// report plus the recipe library hash. Pure of stdout/stderr —
// emitReport handles output.
func runScan(ctx context.Context, cwd string, f scanFlags) (*output.Report, string, error) {
	_ = f.quiet  // reserved: OK findings are not emitted yet.
	_ = f.dryRun // no-op for scan; meaningful for fix.

	repoRoot, err := filepath.Abs(cwd)
	if err != nil {
		return nil, "", &exitErr{code: ExitCrashed, err: fmt.Errorf("abs cwd: %w", err)}
	}
	facts := system.Collect()
	report := output.NewReport(Version, repoRoot, facts.AsSystem())

	lib, err := recipes.DefaultLibrary()
	if err != nil {
		return nil, "", &exitErr{code: ExitCrashed, err: fmt.Errorf("load recipes: %w", err)}
	}
	cfg, err := config.Load(repoRoot, Version)
	if err != nil {
		// Surface the stable error code so CI can tell the difference
		// between a broken machine (codes 1/2) and a broken config (4).
		return nil, "", &exitErr{code: ExitConfigParseError, err: err}
	}

	findings, _ := engine.New(BuiltinProbes(lib, cfg)).Run(ctx, probes.Input{
		RepoRoot: repoRoot,
		System:   facts,
	})
	report.Findings = filterDisabled(findings, cfg)
	report.Finalize()
	return report, lib.Hash(), nil
}

// filterDisabled drops findings whose Probe ID appears in cfg.Disable.
// MVP supports whole-probe disable by ID only; per-inferred-source
// filtering will land once probes start emitting stable inferred IDs.
func filterDisabled(findings []output.Finding, cfg *config.Config) []output.Finding {
	if cfg == nil || len(cfg.Disable) == 0 {
		return findings
	}
	disabled := map[string]bool{}
	for _, id := range cfg.Disable {
		disabled[id] = true
	}
	out := make([]output.Finding, 0, len(findings))
	for _, f := range findings {
		if disabled[f.Probe] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// emitReport renders the report to stdout (TTY or JSON) and, when
// --bundle is set, writes a redacted Bundle to disk with a one-line
// pre-write preview to stderr. stdout receives ONLY the chosen
// representation; everything else goes to stderr so script pipelines
// remain clean.
func emitReport(stdout, stderr io.Writer, r *output.Report, recipeHash string, f scanFlags) error {
	if f.bundle != "" {
		b := bundle.New(Version, r, recipeHash)
		stats, err := bundle.WritePath(f.bundle, b, bundle.RedactOptions{IncludePaths: f.bundleIncludePaths})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stderr, stats.PreviewLine(f.bundle)); err != nil {
			return fmt.Errorf("write preview: %w", err)
		}
	}
	if f.jsonOut {
		if err := output.WriteJSON(stdout, r); err != nil {
			return fmt.Errorf("write json: %w", err)
		}
		return nil
	}
	opts := output.DefaultRenderOptions(os.Stdout)
	if err := output.Render(stdout, r, opts); err != nil {
		return fmt.Errorf("render: %w", err)
	}
	return nil
}
