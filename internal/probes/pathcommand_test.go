// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reswaraa/envdoctor/internal/inference"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/system"
)

const testPathRecipe = `
id: path-command-missing
probe: path-command
fixes:
  - id: brew-install
    class: shared
    when: { has_tool: brew }
    command: "brew install {{.BrewPackage}}"
  - id: apt-install
    class: privileged
    when: { has_tool: apt }
    command: "sudo apt-get install -y {{.AptPackage}}"
    fallback: true
`

func TestPathCommand_ID(t *testing.T) {
	if got := PathCommand(nil).ID(); got != "path-command" {
		t.Errorf("got %q, want path-command", got)
	}
}

func TestPathCommand_AppliesTo(t *testing.T) {
	p := PathCommand(nil)
	if p.AppliesTo(Input{RepoRoot: t.TempDir()}) {
		t.Error("empty repo must not apply")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte("all:\n\tmake build\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !p.AppliesTo(Input{RepoRoot: dir}) {
		t.Error("Makefile-with-make should apply")
	}
}

func TestPathCommand_EvaluateNoFindingsWhenAllPresent(t *testing.T) {
	// Use "go" which is always on PATH in dev/CI as a proxy for a present
	// command. The probe should emit no Finding when HasTool is true.
	p := &pathCommandProbe{lib: nil}
	reqs := []inference.CommandRequirement{
		{Source: "Makefile", Command: "go"},
	}
	if got := p.evaluate(reqs, &system.Facts{}); got != nil {
		t.Errorf("expected nil for all-present; got %+v", got)
	}
}

func TestPathCommand_EvaluateMissingEmitsFinding(t *testing.T) {
	t.Setenv("PATH", "") // make HasTool return false for everything
	p := &pathCommandProbe{lib: nil}
	reqs := []inference.CommandRequirement{
		{Source: "Makefile", Command: "psql"},
		{Source: "Procfile", Command: "redis-cli"},
	}
	got := p.evaluate(reqs, &system.Facts{})
	if len(got) != 2 {
		t.Fatalf("findings: got %d, want 2", len(got))
	}
	for _, f := range got {
		if f.Category != output.CategoryPath {
			t.Errorf("Category: got %q, want path", f.Category)
		}
		if f.RecipeID != "" {
			t.Errorf("nil lib should not attach a recipe; got %q", f.RecipeID)
		}
	}
}

func TestPathCommand_RecipeAttachedForKnownCommands(t *testing.T) {
	t.Setenv("PATH", "") // HasTool returns false for everything except items we stub below

	// Stub a `brew` on PATH so the recipe's `has_tool: brew` matches and
	// the brew-install Fix gets selected.
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "brew"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	lib := mkLibrary(t, testPathRecipe)
	p := &pathCommandProbe{lib: lib}
	reqs := []inference.CommandRequirement{
		{Source: "Makefile", Command: "psql"},
	}
	got := p.evaluate(reqs, &system.Facts{OS: "darwin"})
	if len(got) != 1 {
		t.Fatalf("findings: got %d, want 1", len(got))
	}
	if got[0].RecipeID != "brew-install" {
		t.Errorf("RecipeID: got %q, want brew-install", got[0].RecipeID)
	}
	if !strings.Contains(got[0].RecipeCommand, "libpq") {
		t.Errorf("RecipeCommand should template BrewPackage=libpq for psql; got %q", got[0].RecipeCommand)
	}
}

func TestPathCommand_NoRecipeWhenCommandUnknown(t *testing.T) {
	t.Setenv("PATH", "")
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "brew"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	lib := mkLibrary(t, testPathRecipe)
	p := &pathCommandProbe{lib: lib}
	reqs := []inference.CommandRequirement{
		{Source: "Makefile", Command: "some-obscure-binary"},
	}
	got := p.evaluate(reqs, &system.Facts{OS: "darwin"})
	if len(got) != 1 {
		t.Fatalf("findings: got %d, want 1", len(got))
	}
	if got[0].RecipeID != "" {
		t.Errorf("unknown command must not get a recipe; got RecipeID=%q", got[0].RecipeID)
	}
}
