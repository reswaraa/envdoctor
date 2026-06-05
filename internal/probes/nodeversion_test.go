// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/reswaraa/envdoctor/internal/inference"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/recipes"
	"github.com/reswaraa/envdoctor/internal/system"
)

const testNodeRecipe = `
id: node-version-mismatch
probe: node-version
fixes:
  - id: mise-install-node
    class: safe
    when: { has_tool: mise }
    command: "mise install node@{{.Required}}"
  - id: brew-install-node
    class: shared
    when: { has_tool: brew }
    command: "brew install node@{{.MajorVersion}}"
    fallback: true
`

func mkLibrary(t *testing.T, body string) *recipes.Library {
	t.Helper()
	fsys := fstest.MapFS{
		"library/r.yaml": &fstest.MapFile{Data: []byte(body)},
	}
	lib, err := recipes.LoadFS(fsys, "library")
	if err != nil {
		t.Fatalf("LoadFS: %v", err)
	}
	return lib
}

func TestNodeVersion_ID(t *testing.T) {
	p := NodeVersion(nil)
	if p.ID() != "node-version" {
		t.Errorf("ID: got %q, want %q", p.ID(), "node-version")
	}
}

func TestNodeVersion_AppliesTo(t *testing.T) {
	p := NodeVersion(nil)

	noNode := t.TempDir()
	if p.AppliesTo(Input{RepoRoot: noNode}) {
		t.Error("AppliesTo should be false for a repo with no Node manifests")
	}

	withNVMRC := t.TempDir()
	if err := os.WriteFile(filepath.Join(withNVMRC, ".nvmrc"), []byte("20.10.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !p.AppliesTo(Input{RepoRoot: withNVMRC}) {
		t.Error("AppliesTo should be true once .nvmrc exists")
	}
}

func TestNodeVersion_Evaluate_NoRequirements(t *testing.T) {
	p := &nodeVersionProbe{lib: nil}
	if got := p.evaluate(nil, "20.10.0", &system.Facts{OS: "darwin"}); got != nil {
		t.Errorf("expected nil; got %+v", got)
	}
}

func TestNodeVersion_Evaluate_VersionMatches(t *testing.T) {
	p := &nodeVersionProbe{lib: nil}
	reqs := []inference.NodeRequirement{
		{Source: ".nvmrc", Constraint: "20.10.0", IsExact: true},
	}
	if got := p.evaluate(reqs, "20.10.0", &system.Facts{OS: "darwin"}); got != nil {
		t.Errorf("matching version should produce no findings; got %+v", got)
	}
}

func TestNodeVersion_Evaluate_VersionMismatchEmitsFinding(t *testing.T) {
	lib := mkLibrary(t, testNodeRecipe)
	p := &nodeVersionProbe{lib: lib}
	reqs := []inference.NodeRequirement{
		{Source: ".nvmrc", Constraint: "20.10.0", IsExact: true},
	}
	// Facts where neither mise nor brew is present → no recipe attaches.
	got := p.evaluate(reqs, "18.17.0", &system.Facts{OS: "darwin"})
	if len(got) != 1 {
		t.Fatalf("findings: got %d, want 1", len(got))
	}
	f := got[0]
	if f.Probe != "node-version" || f.Category != output.CategoryRuntime {
		t.Errorf("Probe/Category: %+v", f)
	}
	if f.Status != output.StatusFail || f.Severity != output.SeverityError {
		t.Errorf("Status/Severity: %+v", f)
	}
	if f.Observed != "18.17.0" || f.Expected != "20.10.0" {
		t.Errorf("Observed/Expected: %+v", f)
	}
	if f.DocURL == "" {
		t.Error("DocURL must be set")
	}
}

func TestNodeVersion_Evaluate_NotInstalledEmitsFinding(t *testing.T) {
	lib := mkLibrary(t, testNodeRecipe)
	p := &nodeVersionProbe{lib: lib}
	reqs := []inference.NodeRequirement{
		{Source: ".nvmrc", Constraint: "20.10.0", IsExact: true},
	}
	got := p.evaluate(reqs, "", &system.Facts{OS: "darwin"})
	if len(got) != 1 {
		t.Fatalf("findings: got %d, want 1", len(got))
	}
	if got[0].Observed != "(not installed)" {
		t.Errorf("Observed: %q", got[0].Observed)
	}
}

func TestNodeVersion_Evaluate_PrefersExactRequirementForRecipe(t *testing.T) {
	p := &nodeVersionProbe{lib: mkLibrary(t, testNodeRecipe)}
	reqs := []inference.NodeRequirement{
		// package.json is range-style and appears first.
		{Source: "package.json#engines.node", Constraint: "^20.0.0", IsExact: false},
		// .nvmrc is exact and should win for the "expected" display.
		{Source: ".nvmrc", Constraint: "20.10.0", IsExact: true},
	}
	got := p.evaluate(reqs, "18.17.0", &system.Facts{OS: "darwin"})
	if len(got) != 1 {
		t.Fatalf("findings: got %d, want 1", len(got))
	}
	if got[0].Expected != "20.10.0" {
		t.Errorf("Expected: got %q, want 20.10.0 (exact requirement should win)", got[0].Expected)
	}
}

func TestNodeVersion_Evaluate_RecipeAttachedWhenToolPresent(t *testing.T) {
	// Use "go" as a stand-in for has_tool because go must exist in any
	// envdoctor dev/CI environment, and exercising the real Facts.HasTool
	// path is the no-mock contract from the test plan.
	recipe := `
id: node-version-mismatch
probe: node-version
fixes:
  - id: go-stand-in
    class: safe
    when: { has_tool: go }
    command: "echo install node@{{.Required}}"
`
	p := &nodeVersionProbe{lib: mkLibrary(t, recipe)}
	reqs := []inference.NodeRequirement{
		{Source: ".nvmrc", Constraint: "20.10.0", IsExact: true},
	}
	got := p.evaluate(reqs, "18.17.0", &system.Facts{OS: "darwin"})
	if len(got) != 1 {
		t.Fatalf("findings: got %d", len(got))
	}
	if got[0].RecipeID != "go-stand-in" {
		t.Errorf("RecipeID: got %q, want %q", got[0].RecipeID, "go-stand-in")
	}
	if got[0].RecipeCommand != "echo install node@20.10.0" {
		t.Errorf("RecipeCommand: got %q", got[0].RecipeCommand)
	}
}

func TestNodeVersion_Run_UsesInjectedDetector(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("20.10.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &nodeVersionProbe{
		lib:           nil,
		detectVersion: func(_ context.Context) (string, error) { return "18.17.0", nil },
	}
	findings, err := p.Run(context.Background(), Input{
		RepoRoot: dir,
		System:   &system.Facts{OS: "darwin"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings: got %d, want 1", len(findings))
	}
	if !strings.Contains(findings[0].Summary, "18.17.0") {
		t.Errorf("Summary should include detected version; got %q", findings[0].Summary)
	}
}

func TestNodeVersion_Run_DetectorErrorFallsBackToNotInstalled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("20.10.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &nodeVersionProbe{
		lib:           nil,
		detectVersion: func(_ context.Context) (string, error) { return "", os.ErrNotExist },
	}
	findings, err := p.Run(context.Background(), Input{
		RepoRoot: dir,
		System:   &system.Facts{OS: "darwin"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 || findings[0].Observed != "(not installed)" {
		t.Errorf("expected not-installed finding; got %+v", findings)
	}
}
