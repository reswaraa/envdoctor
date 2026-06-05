// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package recipes

import (
	"strings"
	"testing"
	"testing/fstest"
)

const validRecipe = `
id: node-version-mismatch
probe: node_version
fixes:
  - id: mise-install-node
    class: safe
    when: { os: darwin, has_tool: mise }
    command: "mise install node@{{.Required}}"
  - id: brew-install-node
    class: shared
    when: { os: darwin, has_tool: brew }
    command: "brew install node@{{.MajorVersion}}"
    fallback: true
`

const validEnvRecipe = `
id: env-missing
probe: env_required
fixes:
  - id: copy-env-example
    class: safe
    command: "cp .env.example .env"
`

func TestLoadFS_LoadsValidRecipes(t *testing.T) {
	fsys := fstest.MapFS{
		"library/node-version.yaml": &fstest.MapFile{Data: []byte(validRecipe)},
		"library/env-required.yaml": &fstest.MapFile{Data: []byte(validEnvRecipe)},
	}
	lib, err := LoadFS(fsys, "library")
	if err != nil {
		t.Fatalf("LoadFS: %v", err)
	}
	if len(lib.Recipes) != 2 {
		t.Fatalf("recipes len: got %d, want 2", len(lib.Recipes))
	}
	// Sort by ID puts env-missing first.
	if lib.Recipes[0].ID != "env-missing" {
		t.Errorf("Recipes[0].ID: got %q, want %q", lib.Recipes[0].ID, "env-missing")
	}
	if lib.Recipes[1].ID != "node-version-mismatch" {
		t.Errorf("Recipes[1].ID: got %q, want %q", lib.Recipes[1].ID, "node-version-mismatch")
	}
}

func TestLibrary_LookupAndForProbe(t *testing.T) {
	fsys := fstest.MapFS{
		"library/node.yaml": &fstest.MapFile{Data: []byte(validRecipe)},
		"library/env.yaml":  &fstest.MapFile{Data: []byte(validEnvRecipe)},
	}
	lib, err := LoadFS(fsys, "library")
	if err != nil {
		t.Fatalf("LoadFS: %v", err)
	}

	r, ok := lib.Lookup("node-version-mismatch")
	if !ok {
		t.Fatal("Lookup(node-version-mismatch): not found")
	}
	if len(r.Fixes) != 2 {
		t.Errorf("Fixes: got %d, want 2", len(r.Fixes))
	}

	forNode := lib.ForProbe("node_version")
	if len(forNode) != 1 || forNode[0].ID != "node-version-mismatch" {
		t.Errorf("ForProbe(node_version): got %+v", forNode)
	}

	if _, ok := lib.Lookup("nope"); ok {
		t.Error("Lookup of unknown id should return false")
	}
	if got := lib.ForProbe("unknown"); len(got) != 0 {
		t.Errorf("ForProbe(unknown): got %d, want 0", len(got))
	}
}

func TestLoadFS_MalformedYAMLReportsFile(t *testing.T) {
	fsys := fstest.MapFS{
		"library/bad.yaml": &fstest.MapFile{Data: []byte("id: a\nfixes:\n  - not: a: sequence")},
	}
	_, err := LoadFS(fsys, "library")
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
	if !strings.Contains(err.Error(), "bad.yaml") {
		t.Errorf("error should mention the file; got: %v", err)
	}
}

func TestLoadFS_DuplicateRecipeIDRejected(t *testing.T) {
	dup := validRecipe // same id in two files
	fsys := fstest.MapFS{
		"library/a.yaml": &fstest.MapFile{Data: []byte(dup)},
		"library/b.yaml": &fstest.MapFile{Data: []byte(dup)},
	}
	_, err := LoadFS(fsys, "library")
	if err == nil {
		t.Fatal("expected error for duplicate recipe id")
	}
	if !strings.Contains(err.Error(), "duplicate recipe id") {
		t.Errorf("error should mention duplicate; got: %v", err)
	}
}

func TestLoadFS_DuplicateFixIDRejected(t *testing.T) {
	src := `
id: x
probe: y
fixes:
  - id: same
    class: safe
    command: "true"
  - id: same
    class: safe
    command: "true"
`
	fsys := fstest.MapFS{"library/r.yaml": &fstest.MapFile{Data: []byte(src)}}
	_, err := LoadFS(fsys, "library")
	if err == nil {
		t.Fatal("expected error for duplicate fix id")
	}
	if !strings.Contains(err.Error(), "duplicate fix id") {
		t.Errorf("error should mention duplicate fix; got: %v", err)
	}
}

func TestLoadFS_MissingRequiredFields(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{
			name: "missing id",
			body: "probe: x\nfixes:\n  - id: a\n    class: safe\n    command: 'true'\n",
			want: "missing 'id'",
		},
		{
			name: "missing probe",
			body: "id: r\nfixes:\n  - id: a\n    class: safe\n    command: 'true'\n",
			want: "missing 'probe'",
		},
		{
			name: "empty fixes",
			body: "id: r\nprobe: x\nfixes: []\n",
			want: "has no 'fixes'",
		},
		{
			name: "fix missing id",
			body: "id: r\nprobe: x\nfixes:\n  - class: safe\n    command: 'true'\n",
			want: "fix #1 missing 'id'",
		},
		{
			name: "fix missing command",
			body: "id: r\nprobe: x\nfixes:\n  - id: a\n    class: safe\n",
			want: "missing 'command'",
		},
		{
			name: "fix missing class",
			body: "id: r\nprobe: x\nfixes:\n  - id: a\n    command: 'true'\n",
			want: "missing 'class'",
		},
		{
			name: "fix unknown class",
			body: "id: r\nprobe: x\nfixes:\n  - id: a\n    class: nuclear\n    command: 'true'\n",
			want: "unknown class",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fsys := fstest.MapFS{"library/r.yaml": &fstest.MapFile{Data: []byte(c.body)}}
			_, err := LoadFS(fsys, "library")
			if err == nil {
				t.Fatalf("expected error containing %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q must contain %q", err.Error(), c.want)
			}
		})
	}
}

func TestDefaultLibrary_LoadsEmbedded(t *testing.T) {
	// At Phase 2A there are no real recipes yet; the embedded library
	// is empty. We only assert that DefaultLibrary returns without
	// error and that the embed wiring works.
	_, err := DefaultLibrary()
	if err != nil {
		t.Fatalf("DefaultLibrary: %v", err)
	}
}
