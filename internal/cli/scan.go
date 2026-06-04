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

	"github.com/reswaraa/envdoctor/internal/engine"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/probes"
	"github.com/reswaraa/envdoctor/internal/system"
)

type scanFlags struct {
	jsonOut bool
	quiet   bool
	bundle  string
	dryRun  bool
}

func newScanCmd() *cobra.Command {
	var f scanFlags
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan the current repo for environment problems",
		Long: `Scan inspects the current directory for known manifest files,
probes the local system, and reports findings with copy-pasteable repair
commands.

Phase 1 wires the engine and output paths but ships no probes yet, so a
scan returns a clean report. Probes land in Phase 2.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return &exitErr{code: ExitCrashed, err: fmt.Errorf("resolve cwd: %w", err)}
			}
			report, runErr := runScan(cmd.Context(), cwd, f)
			if runErr != nil {
				return runErr
			}
			if err := emitReport(cmd.OutOrStdout(), report, f); err != nil {
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
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "no-op for scan; honored by `envdoctor fix`")
	return cmd
}

// runScan collects facts, runs the engine, and returns the populated
// report. Pure of stdout/stderr — emitReport handles output.
func runScan(ctx context.Context, cwd string, f scanFlags) (*output.Report, error) {
	_ = f.quiet  // reserved for Phase 1; OK findings are not emitted yet.
	_ = f.dryRun // no-op for scan; meaningful for fix.

	repoRoot, err := filepath.Abs(cwd)
	if err != nil {
		return nil, &exitErr{code: ExitCrashed, err: fmt.Errorf("abs cwd: %w", err)}
	}
	facts := system.Collect()
	report := output.NewReport(Version, repoRoot, facts.AsSystem())

	findings, _ := engine.New(nil).Run(ctx, probes.Input{
		RepoRoot: repoRoot,
		System:   facts,
	})
	report.Findings = findings
	report.Finalize()
	return report, nil
}

func emitReport(stdout io.Writer, r *output.Report, f scanFlags) error {
	if f.bundle != "" {
		bf, err := os.Create(f.bundle)
		if err != nil {
			return fmt.Errorf("create bundle: %w", err)
		}
		defer func() { _ = bf.Close() }()
		if err := output.WriteJSON(bf, r); err != nil {
			return fmt.Errorf("write bundle: %w", err)
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
