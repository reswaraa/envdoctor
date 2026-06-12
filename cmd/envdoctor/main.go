// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Command envdoctor is the entrypoint binary; all real logic lives in
// internal/cli. Build with -ldflags "-X .../internal/cli.Version=..."
// to inject release metadata.
package main

import (
	"os"

	"github.com/reswaraa/envdoctor/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
