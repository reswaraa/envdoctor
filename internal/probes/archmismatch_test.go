// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/reswaraa/envdoctor/internal/inference"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/system"
)

const testArchRecipe = `
id: native-arm64-missing
probe: arch-mismatch
fixes:
  - id: bump-to-arm-compatible
    class: safe
    command: "npm install {{.Name}}@^{{.FixedVersion}}"
`

func TestArchMismatch_ID(t *testing.T) {
	if got := ArchMismatch(nil).ID(); got != "arch-mismatch" {
		t.Errorf("got %q, want arch-mismatch", got)
	}
}

func TestArchMismatch_AppliesTo(t *testing.T) {
	p := ArchMismatch(nil)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// arm64 + lockfile → applies
	if !p.AppliesTo(Input{RepoRoot: dir, System: &system.Facts{Arch: "arm64"}}) {
		t.Error("arm64 + lockfile must apply")
	}
	// amd64 + lockfile → does NOT apply (x86 systems don't hit the issue)
	if p.AppliesTo(Input{RepoRoot: dir, System: &system.Facts{Arch: "amd64"}}) {
		t.Error("amd64 must not apply (probe is arm64-only)")
	}
	// arm64 + no lockfile → does NOT apply
	if p.AppliesTo(Input{RepoRoot: t.TempDir(), System: &system.Facts{Arch: "arm64"}}) {
		t.Error("arm64 alone must not apply")
	}
}

func TestArchMismatch_Evaluate_FlagsBadVersions(t *testing.T) {
	p := &archMismatchProbe{lib: mkLibrary(t, testArchRecipe)}
	deps := []inference.NativeDep{
		{Source: "package-lock.json", Name: "sharp", Version: "0.31.0"},
		{Source: "package-lock.json", Name: "cypress", Version: "12.17.0"},
	}
	got := p.evaluate(deps, &system.Facts{Arch: "arm64"})
	if len(got) != 2 {
		t.Fatalf("findings: got %d, want 2", len(got))
	}
	if got[0].Category != output.CategoryArchitecture {
		t.Errorf("Category: got %q", got[0].Category)
	}
	if got[0].Severity != output.SeverityWarning {
		t.Errorf("severity should be warning (arch issues are often tolerable); got %q", got[0].Severity)
	}
	if got[0].RecipeID != "bump-to-arm-compatible" {
		t.Errorf("RecipeID: got %q", got[0].RecipeID)
	}
	// Template expansion: command must mention the dep name and the
	// fixed-version target.
	if got[0].RecipeCommand != "npm install sharp@^0.33.0" {
		t.Errorf("RecipeCommand: got %q", got[0].RecipeCommand)
	}
}

func TestArchMismatch_Evaluate_PassesFixedVersions(t *testing.T) {
	p := &archMismatchProbe{lib: nil}
	deps := []inference.NativeDep{
		{Source: "package-lock.json", Name: "sharp", Version: "0.33.5"},
		{Source: "package-lock.json", Name: "cypress", Version: "13.6.0"},
	}
	if got := p.evaluate(deps, &system.Facts{Arch: "arm64"}); got != nil {
		t.Errorf("expected no findings for fixed versions; got %+v", got)
	}
}

func TestArchMismatch_Evaluate_IgnoresUnknownDeps(t *testing.T) {
	p := &archMismatchProbe{lib: nil}
	deps := []inference.NativeDep{
		{Source: "package-lock.json", Name: "some-other-native-pkg", Version: "0.1.0"},
	}
	if got := p.evaluate(deps, &system.Facts{Arch: "arm64"}); got != nil {
		t.Errorf("unknown native deps must not surface; got %+v", got)
	}
}

func TestArchMismatch_Evaluate_HandlesUnparseableVersion(t *testing.T) {
	p := &archMismatchProbe{lib: nil}
	deps := []inference.NativeDep{
		{Source: "package-lock.json", Name: "sharp", Version: "git+ssh://..."},
	}
	// semver.Satisfies returns error for non-numeric versions; the probe
	// silently skips (rather than emit a noisy "couldn't parse" finding).
	if got := p.evaluate(deps, &system.Facts{Arch: "arm64"}); got != nil {
		t.Errorf("unparseable version must be silently skipped; got %+v", got)
	}
}
