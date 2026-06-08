// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/reswaraa/envdoctor/internal/recipes"
)

func TestBuiltinProbes_ContainsExpectedIDs(t *testing.T) {
	lib, err := recipes.DefaultLibrary()
	if err != nil {
		t.Fatalf("DefaultLibrary: %v", err)
	}
	ps := BuiltinProbes(lib)
	if len(ps) != 5 {
		t.Fatalf("expected 5 probes; got %d", len(ps))
	}
	ids := make([]string, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, p.ID())
	}
	sort.Strings(ids)
	want := []string{"docker-running", "env-required", "node-version", "path-command", "port-free"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("probe IDs: got %v, want %v", ids, want)
	}
}

func TestBuiltinProbes_NilLibAllowed(t *testing.T) {
	ps := BuiltinProbes(nil)
	if len(ps) != 5 {
		t.Fatalf("expected 5 probes even with nil lib; got %d", len(ps))
	}
}

// TestScan_OnNodeAppFixture_RunsCleanly exercises the full scan path
// against the testdata/repos/node-app/ fixture. Findings emitted are
// host-dependent (the host may or may not have Node 20 installed,
// port 5432 may or may not be free, POSTGRES_USER may or may not be
// in the process env). We assert only structural properties that
// must hold regardless of host state.
func TestScan_OnNodeAppFixture_RunsCleanly(t *testing.T) {
	const fixture = "../../testdata/repos/node-app"
	report, err := runScan(context.Background(), fixture, scanFlags{})
	if err != nil {
		t.Fatalf("runScan: %v", err)
	}
	if report.SchemaVersion == "" {
		t.Error("Report.SchemaVersion must be set")
	}
	if report.RepoRoot == "" {
		t.Error("Report.RepoRoot must be set")
	}
	for _, f := range report.Findings {
		if f.ID == "" {
			t.Errorf("finding has no ID: %+v", f)
		}
		if f.Probe == "" {
			t.Errorf("finding has no Probe: %+v", f)
		}
		if f.DocURL == "" {
			t.Errorf("finding %s has no DocURL", f.ID)
		}
		if f.Category == "" {
			t.Errorf("finding %s has no Category", f.ID)
		}
	}
}
