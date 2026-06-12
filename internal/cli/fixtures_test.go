// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/reswaraa/envdoctor/internal/probes"
	"github.com/reswaraa/envdoctor/internal/recipes"
	"github.com/reswaraa/envdoctor/internal/system"
)

// TestScan_FixtureRepos is the structural E2E smoke for every shipped
// fixture under testdata/repos/. It does NOT pin a golden JSON: the
// per-Finding output depends on host state (installed Node/Python/Go
// versions, port 5432 ownership, Docker daemon status, $POSTGRES_USER
// in the test runner's env, etc.). Instead it asserts:
//
//  1. The expected set of probes applies to each fixture (a structural
//     property of the fixture, host-independent).
//  2. Every emitted Finding is well-formed (ID, Probe, Category, and
//     DocURL set — CI fails the build if any DocURL 404s on the docs site).
func TestScan_FixtureRepos(t *testing.T) {
	cases := []struct {
		name        string
		expectApply []string
	}{
		{
			name: "node-app",
			expectApply: []string{
				"node-version",
				"env-required",
				"port-free",
				"docker-running",
			},
		},
		{
			name: "python-app",
			expectApply: []string{
				"python-version",
				"env-required",
			},
		},
		{
			name: "go-module",
			expectApply: []string{
				"go-version",
				"path-command",
			},
		},
		{
			name: "ruby-app",
			expectApply: []string{
				"ruby-version",
				"env-required",
			},
		},
		{
			name: "compose-app",
			expectApply: []string{
				"env-required",
				"port-free",
				"docker-running",
				"path-command",
			},
		},
	}

	lib, err := recipes.DefaultLibrary()
	if err != nil {
		t.Fatalf("DefaultLibrary: %v", err)
	}
	ps := BuiltinProbes(lib, nil)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fixture := filepath.Join("..", "..", "testdata", "repos", c.name)

			in := probes.Input{
				RepoRoot: fixture,
				System:   system.Collect(),
			}
			applying := map[string]bool{}
			for _, p := range ps {
				if p.AppliesTo(in) {
					applying[p.ID()] = true
				}
			}
			for _, want := range c.expectApply {
				if !applying[want] {
					t.Errorf("probe %q should apply to %s; applying=%v", want, c.name, applying)
				}
			}

			report, _, err := runScan(context.Background(), fixture, scanFlags{})
			if err != nil {
				t.Fatalf("runScan: %v", err)
			}
			if report.SchemaVersion == "" {
				t.Error("SchemaVersion must be set")
			}
			if report.RepoRoot == "" {
				t.Error("RepoRoot must be set")
			}
			for _, f := range report.Findings {
				if f.ID == "" {
					t.Errorf("finding missing ID: %+v", f)
				}
				if f.Probe == "" {
					t.Errorf("finding missing Probe: %+v", f)
				}
				if f.Category == "" {
					t.Errorf("finding %q missing Category", f.ID)
				}
				if f.DocURL == "" {
					t.Errorf("finding %q (probe %q) missing DocURL", f.ID, f.Probe)
				}
			}
		})
	}
}
