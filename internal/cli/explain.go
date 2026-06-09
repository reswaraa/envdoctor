// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/reswaraa/envdoctor/internal/bundle"
	"github.com/reswaraa/envdoctor/internal/output"
)

func newExplainCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "explain <bundle.json>",
		Short: "Re-render a debug bundle as if the scan had just happened",
		Long: `Explain reads a bundle produced by ` + "`envdoctor scan --bundle`" + ` and
renders the embedded Report. Useful for a maintainer triaging an issue
where the contributor attached a bundle.json — no need to repro the
contributor's environment.

The bundle's recipe_hash is reported alongside the rendering so the
maintainer can tell whether their local envdoctor would have
produced the same advice or a drifted one.`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			b, err := bundle.Read(path)
			if err != nil {
				return &exitErr{code: ExitCrashed, err: err}
			}
			if b.Report == nil {
				return &exitErr{code: ExitCrashed, err: fmt.Errorf("%s: bundle has no report", path)}
			}
			if jsonOut {
				if err := output.WriteJSON(cmd.OutOrStdout(), b.Report); err != nil {
					return &exitErr{code: ExitCrashed, err: err}
				}
				return nil
			}
			opts := output.DefaultRenderOptions(os.Stdout)
			if err := output.Render(cmd.OutOrStdout(), b.Report, opts); err != nil {
				return &exitErr{code: ExitCrashed, err: err}
			}
			if b.RecipeHash != "" {
				short := b.RecipeHash
				if len(short) > 12 {
					short = short[:12]
				}
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(),
					"\nBundle recipe_hash: %s (envdoctor %s)\n",
					short, b.EnvdoctorVersion,
				); err != nil {
					return &exitErr{code: ExitCrashed, err: err}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the embedded Report as JSON instead of the pretty TTY view")
	return cmd
}
