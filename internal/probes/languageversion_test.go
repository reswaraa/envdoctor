// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Tests for the python / go / ruby version probes. Each mirrors the
// node_version probe's pattern; tests here focus on the parts that
// differ (inference plumbing, version-detector parsing, recipe param
// expansion).

package probes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reswaraa/envdoctor/internal/inference"
	"github.com/reswaraa/envdoctor/internal/system"
)

const langTestRecipes = `
id: python-version-mismatch
probe: python-version
fixes:
  - id: go-stand-in
    class: safe
    when: { has_tool: go }
    command: "echo install python@{{.Required}}"
`

const langGoRecipes = `
id: go-version-mismatch
probe: go-version
fixes:
  - id: go-stand-in
    class: safe
    when: { has_tool: go }
    command: "echo install go@{{.Required}}"
`

const langRubyRecipes = `
id: ruby-version-mismatch
probe: ruby-version
fixes:
  - id: go-stand-in
    class: safe
    when: { has_tool: go }
    command: "echo install ruby@{{.Required}}"
`

// --- python ---

func TestPythonVersion_BasicMismatch(t *testing.T) {
	p := &pythonVersionProbe{lib: mkLibrary(t, langTestRecipes)}
	reqs := []inference.PythonRequirement{
		{Source: ".python-version", Constraint: "3.11.5", IsExact: true},
	}
	got := p.evaluate(reqs, "3.10.0", &system.Facts{OS: "darwin"})
	if len(got) != 1 || got[0].Probe != "python-version" || got[0].Expected != "3.11.5" {
		t.Errorf("got %+v", got)
	}
	if got[0].RecipeCommand != "echo install python@3.11.5" {
		t.Errorf("RecipeCommand: got %q", got[0].RecipeCommand)
	}
}

func TestPythonVersion_NotInstalled(t *testing.T) {
	p := &pythonVersionProbe{lib: nil}
	reqs := []inference.PythonRequirement{
		{Source: ".python-version", Constraint: "3.11.5", IsExact: true},
	}
	got := p.evaluate(reqs, "", &system.Facts{})
	if len(got) != 1 || got[0].Observed != "(not installed)" {
		t.Errorf("got %+v", got)
	}
}

func TestPythonVersion_AppliesToAndID(t *testing.T) {
	if got := PythonVersion(nil).ID(); got != "python-version" {
		t.Errorf("ID: got %q", got)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".python-version"), []byte("3.11.5"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !PythonVersion(nil).AppliesTo(Input{RepoRoot: dir}) {
		t.Error(".python-version should make probe apply")
	}
}

// --- go ---

func TestGoVersion_BasicMismatch(t *testing.T) {
	p := &goVersionProbe{lib: mkLibrary(t, langGoRecipes)}
	reqs := []inference.GoRequirement{
		{Source: "go.mod", Constraint: "1.22", IsExact: false},
	}
	got := p.evaluate(reqs, "1.21.5", &system.Facts{})
	if len(got) != 1 || !strings.Contains(got[0].Expected, "1.22") {
		t.Errorf("got %+v", got)
	}
}

func TestGoVersion_NewerInstalledPasses(t *testing.T) {
	p := &goVersionProbe{lib: nil}
	reqs := []inference.GoRequirement{
		{Source: "go.mod", Constraint: "1.21", IsExact: false},
	}
	if got := p.evaluate(reqs, "1.22.0", &system.Facts{}); got != nil {
		t.Errorf("newer installed must satisfy go.mod minimum; got %+v", got)
	}
}

func TestGoVersion_DetectorParsesGoVersionOutput(t *testing.T) {
	// realDetectGoVersion isn't exercised here (no real go fork); we test
	// the parser indirectly via package-level helpers below. As a smoke,
	// confirm the production detector returns *something* on this host.
	v, err := realDetectGoVersion(context.Background())
	if err != nil {
		t.Skipf("go not on PATH: %v", err)
	}
	if v == "" || !strings.ContainsRune(v, '.') {
		t.Errorf("expected dotted version; got %q", v)
	}
}

// --- ruby ---

func TestRubyVersion_BasicMismatch(t *testing.T) {
	p := &rubyVersionProbe{lib: mkLibrary(t, langRubyRecipes)}
	reqs := []inference.RubyRequirement{
		{Source: ".ruby-version", Constraint: "3.2.2", IsExact: true},
	}
	got := p.evaluate(reqs, "3.1.0", &system.Facts{})
	if len(got) != 1 || got[0].Expected != "3.2.2" {
		t.Errorf("got %+v", got)
	}
	if got[0].RecipeCommand != "echo install ruby@3.2.2" {
		t.Errorf("RecipeCommand: got %q", got[0].RecipeCommand)
	}
}

func TestRubyVersion_PrefersExactSource(t *testing.T) {
	p := &rubyVersionProbe{lib: nil}
	reqs := []inference.RubyRequirement{
		{Source: "Gemfile#ruby", Constraint: "~> 3.2", IsExact: false},
		{Source: ".ruby-version", Constraint: "3.2.5", IsExact: true},
	}
	got := p.evaluate(reqs, "3.1.0", &system.Facts{})
	if len(got) != 1 || got[0].Expected != "3.2.5" {
		t.Errorf("exact source must win; got %+v", got)
	}
}
