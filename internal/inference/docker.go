// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"os"
	"path/filepath"
)

// dockerSignalFiles is the set of repo files whose presence indicates
// the repo expects Docker to be available locally. Order is the
// fixture-friendly priority (Dockerfile is canonical; compose files
// follow).
var dockerSignalFiles = []string{
	"Dockerfile",
	"docker-compose.yml",
	"docker-compose.yaml",
	"compose.yml",
	"compose.yaml",
}

// HasDockerSignals reports whether the repo at root has any signal
// that suggests Docker is expected to be running locally. Used by
// the docker_running probe's AppliesTo.
func HasDockerSignals(root string) bool {
	for _, name := range dockerSignalFiles {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}
