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
	return cmd
}

// Execute runs the root command and returns the process exit code.
// Phase 0 maps any error to exit code 3 (envdoctor itself crashed). The
// full exit-code matrix from implementation.md Q10 is wired in Phase 1.
func Execute() int {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "envdoctor:", err)
		return 3
	}
	return 0
}
