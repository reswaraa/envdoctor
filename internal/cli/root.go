// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package cli wires the cobra command tree for the envdoctor CLI.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Build-time values injected via -ldflags by GoReleaser.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "envdoctor",
		Short: "Diagnose why a repo will not run on your machine",
		Long: `EnvDoctor scans your local runtime state, compares it against
repo requirements (.nvmrc, package.json, docker-compose.yml, pyproject.toml,
...), and emits copy-pasteable fixes for what is broken.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newScanCmd())
	cmd.AddCommand(newFixCmd())
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newLintCmd())
	cmd.AddCommand(newExplainCmd())
	return cmd
}

// Execute runs the root command and returns the process exit code.
// Errors carrying an ExitCoder use its code; anything else is treated
// as ExitCrashed and its message goes to stderr.
func Execute() int {
	err := newRootCmd().Execute()
	if err == nil {
		return ExitOK
	}
	if code, ok := asExitCode(err); ok {
		if msg := err.Error(); msg != "" {
			fmt.Fprintln(os.Stderr, "envdoctor:", msg)
		}
		return code
	}
	fmt.Fprintln(os.Stderr, "envdoctor:", err)
	return ExitCrashed
}
