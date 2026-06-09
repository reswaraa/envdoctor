// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/reswaraa/envdoctor/internal/config"
)

func newLintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lint",
		Short: "Validate the repo's .envdoctor.yaml against the schema",
		Long: `Lint reads .envdoctor.yaml from the current directory and validates
it against the embedded schema.

Stable error codes (E001-E010) appear in the output so CI scripts and
editor language servers can grep for them. Exit codes:

    0   config is valid (or no config present)
    3   envdoctor itself crashed
    4   config is malformed`,
		// Match the root command — envdoctor owns its error rendering;
		// cobra must not print "Error: …" or the usage block on top.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runLint,
	}
}

func runLint(cmd *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return &exitErr{code: ExitCrashed, err: fmt.Errorf("resolve cwd: %w", err)}
	}
	cfg, err := config.Load(cwd, Version)
	if err != nil {
		var ce *config.Error
		if errors.As(err, &ce) {
			// Note: cobra's `cmd.OutOrStderr()` is a misnomer — it
			// returns the stdout writer with os.Stderr as a fallback.
			// For an actual stderr write, use `cmd.ErrOrStderr()`.
			if _, werr := fmt.Fprintf(cmd.ErrOrStderr(), "%s\n", ce.Error()); werr != nil {
				return &exitErr{code: ExitCrashed, err: werr}
			}
			return &exitErr{code: ExitConfigParseError}
		}
		return &exitErr{code: ExitCrashed, err: err}
	}
	out := cmd.OutOrStdout()
	if cfg == nil {
		_, err := fmt.Fprintf(out, "no %s in %s — config is optional, nothing to lint\n", config.FileName, filepath.Base(cwd))
		if err != nil {
			return &exitErr{code: ExitCrashed, err: err}
		}
		return nil
	}
	_, err = fmt.Fprintf(out, "ok  %s  (schema_version: %d, %d check(s), %d override(s), %d disable(s))\n",
		config.FileName,
		cfg.SchemaVersion,
		len(cfg.Checks),
		len(cfg.Overrides),
		len(cfg.Disable),
	)
	if err != nil {
		return &exitErr{code: ExitCrashed, err: err}
	}
	return nil
}
