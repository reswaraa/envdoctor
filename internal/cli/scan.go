// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Scan the current repo for environment problems",
		Long: `Scan inspects the current directory for known manifest files,
probes the local system, and reports findings with copy-pasteable repair
commands.

Stub in Phase 0; the engine, probes, and output renderer are wired in
Phase 1.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(
				cmd.OutOrStderr(),
				"envdoctor scan: stub (Phase 1 wires the engine and probes).",
			)
			return err
		},
	}
}
