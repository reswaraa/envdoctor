// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reswaraa/envdoctor/internal/recipes"
)

// --- summarizeMatch -----------------------------------------------

func TestSummarizeMatch(t *testing.T) {
	cases := []struct {
		m    recipes.Match
		want string
	}{
		{recipes.Match{}, "*"},
		{recipes.Match{HasTool: "mise"}, "has_tool=mise"},
		{recipes.Match{OS: "darwin", HasTool: "brew"}, "os=darwin, has_tool=brew"},
		{recipes.Match{OS: "linux", Arch: "arm64", Distro: "ubuntu", HasTool: "apt"},
			"os=linux, arch=arm64, distro=ubuntu, has_tool=apt"},
	}
	for _, c := range cases {
		if got := summarizeMatch(c.m); got != c.want {
			t.Errorf("summarizeMatch(%+v): got %q, want %q", c.m, got, c.want)
		}
	}
}

// --- renderProbeRecipes -------------------------------------------

func TestRenderProbeRecipes_EmptyShowsPlaceholder(t *testing.T) {
	got := renderProbeRecipes(nil)
	if !strings.Contains(got, "No recipes today") {
		t.Errorf("empty render should show the placeholder; got %q", got)
	}
}

func TestRenderProbeRecipes_SingleRecipeNoHeading(t *testing.T) {
	rs := []recipes.Recipe{{
		ID:    "env-missing",
		Probe: "env-required",
		Fixes: []recipes.Fix{
			{ID: "copy-env-example", Class: recipes.ClassSafe},
		},
	}}
	got := renderProbeRecipes(rs)
	// Single recipe → no `### <id>` heading; just the table.
	if strings.Contains(got, "### `env-missing`") {
		t.Errorf("single recipe should NOT emit a heading; got:\n%s", got)
	}
	for _, want := range []string{
		"| Fix | Class | When | Fallback |",
		"| `copy-env-example` | safe | * |  |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing line %q in:\n%s", want, got)
		}
	}
}

func TestRenderProbeRecipes_MultipleRecipesEachGetHeading(t *testing.T) {
	rs := []recipes.Recipe{
		{
			ID: "docker-cli-missing", Probe: "docker-running",
			Fixes: []recipes.Fix{
				{ID: "brew-cask-docker", Class: recipes.ClassShared,
					When: recipes.Match{OS: "darwin", HasTool: "brew"}},
			},
		},
		{
			ID: "docker-daemon-down", Probe: "docker-running",
			Fixes: []recipes.Fix{
				{ID: "colima-start", Class: recipes.ClassSafe,
					When: recipes.Match{HasTool: "colima"}},
			},
		},
	}
	got := renderProbeRecipes(rs)
	for _, want := range []string{
		"### `docker-cli-missing`",
		"### `docker-daemon-down`",
		"| `brew-cask-docker` | shared | os=darwin, has_tool=brew |  |",
		"| `colima-start` | safe | has_tool=colima |  |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderProbeRecipes_FallbackMarked(t *testing.T) {
	rs := []recipes.Recipe{{
		ID:    "node-version-mismatch",
		Probe: "node-version",
		Fixes: []recipes.Fix{
			{ID: "mise", Class: recipes.ClassSafe, When: recipes.Match{HasTool: "mise"}},
			{ID: "brew", Class: recipes.ClassShared, When: recipes.Match{HasTool: "brew"}, Fallback: true},
		},
	}}
	got := renderProbeRecipes(rs)
	// The first row has Fallback="" (empty), the second has "yes".
	if !strings.Contains(got, "| `mise` | safe | has_tool=mise |  |") {
		t.Errorf("non-fallback row mismatch; got:\n%s", got)
	}
	if !strings.Contains(got, "| `brew` | shared | has_tool=brew | yes |") {
		t.Errorf("fallback row mismatch; got:\n%s", got)
	}
}

// --- applyMarkers -------------------------------------------------

func TestApplyMarkers_ReplaceBetweenMarkers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "probe.md")
	if err := os.WriteFile(p, []byte(`# probe

intro text

<!-- BEGIN auto-recipes -->
OLD TABLE
<!-- END auto-recipes -->

trailing text
`), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, ok, err := applyMarkers(p, "NEW TABLE", false)
	if err != nil {
		t.Fatalf("applyMarkers: %v", err)
	}
	if !ok {
		t.Fatalf("ok should be true when markers present")
	}
	if !changed {
		t.Errorf("changed should be true when body differs")
	}

	got, _ := os.ReadFile(p)
	for _, want := range []string{
		"# probe",       // header preserved
		"intro text",    // intro preserved
		"trailing text", // trailing preserved
		"<!-- BEGIN auto-recipes -->",
		"NEW TABLE",
		"<!-- END auto-recipes -->",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q after replace; got:\n%s", want, string(got))
		}
	}
	if strings.Contains(string(got), "OLD TABLE") {
		t.Errorf("OLD TABLE should have been replaced; got:\n%s", string(got))
	}
}

func TestApplyMarkers_NoChangeWhenBodyMatches(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "probe.md")
	original := "# probe\n\n<!-- BEGIN auto-recipes -->\n\nBODY\n<!-- END auto-recipes -->\n"
	if err := os.WriteFile(p, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, ok, err := applyMarkers(p, "BODY", false)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok should be true")
	}
	if changed {
		t.Errorf("changed should be false when body identical")
	}
}

func TestApplyMarkers_DryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "probe.md")
	original := "<!-- BEGIN auto-recipes -->\nOLD\n<!-- END auto-recipes -->\n"
	if err := os.WriteFile(p, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, ok, err := applyMarkers(p, "NEW", true)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !changed {
		t.Errorf("expected ok=true changed=true; got ok=%v changed=%v", ok, changed)
	}
	got, _ := os.ReadFile(p)
	if string(got) != original {
		t.Errorf("dry-run wrote to disk! got:\n%s", string(got))
	}
}

func TestApplyMarkers_MissingMarkersReturnsNotOk(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "probe.md")
	if err := os.WriteFile(p, []byte("# no markers here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, ok, err := applyMarkers(p, "BODY", false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Errorf("ok must be false when markers absent")
	}
}

// --- end-to-end against the real library + docs -------------------

// TestLibraryAndDocsAreInSync regenerates against the real
// embedded library and asserts the on-disk probe pages match.
// This is the regression net for "someone changed a YAML class
// but forgot to re-run go run ./scripts/recipes-to-mdx".
//
// Equivalent to running `go run ./scripts/recipes-to-mdx -check`
// in CI but reachable from `go test ./...`.
func TestLibraryAndDocsAreInSync(t *testing.T) {
	root := repoRoot(t)
	probesDir := filepath.Join(root, "docs", "src", "content", "docs", "probes")
	lib, err := recipes.DefaultLibrary()
	if err != nil {
		t.Fatalf("DefaultLibrary: %v", err)
	}
	byProbe := map[string][]recipes.Recipe{}
	for _, r := range lib.Recipes {
		byProbe[r.Probe] = append(byProbe[r.Probe], r)
	}
	files, err := filepath.Glob(filepath.Join(probesDir, "*.md"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no probe pages found under %s", probesDir)
	}
	for _, f := range files {
		probeID := strings.TrimSuffix(filepath.Base(f), ".md")
		body := renderProbeRecipes(byProbe[probeID])
		changed, ok, err := applyMarkers(f, body, true) // dry-run
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if !ok {
			t.Errorf("%s: missing auto-recipes markers", f)
			continue
		}
		if changed {
			rel, _ := filepath.Rel(root, f)
			t.Errorf("%s out of sync — run `go run ./scripts/recipes-to-mdx`", rel)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not find go.mod above %s", dir)
	return ""
}
