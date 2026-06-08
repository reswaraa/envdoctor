// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasDockerSignals(t *testing.T) {
	t.Run("empty repo", func(t *testing.T) {
		if HasDockerSignals(t.TempDir()) {
			t.Error("empty repo must not signal Docker")
		}
	})

	for _, name := range []string{
		"Dockerfile",
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, name, "")
			if !HasDockerSignals(dir) {
				t.Errorf("%s should signal Docker", name)
			}
		})
	}

	t.Run("unrelated file", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "package.json", "{}")
		if HasDockerSignals(dir) {
			t.Error("package.json alone must not signal Docker")
		}
	})

	t.Run("nested Dockerfile ignored", func(t *testing.T) {
		// HasDockerSignals scans the repo root, not the whole tree.
		dir := t.TempDir()
		sub := filepath.Join(dir, "nested")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, sub, "Dockerfile", "")
		if HasDockerSignals(dir) {
			t.Error("nested Dockerfile must not match repo-root scan")
		}
	})
}
