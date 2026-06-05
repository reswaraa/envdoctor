// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestInferNode_EmptyRepoReturnsNothing(t *testing.T) {
	dir := t.TempDir()
	reqs, err := InferNode(dir)
	if err != nil {
		t.Fatalf("InferNode: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("expected 0 requirements; got %d: %+v", len(reqs), reqs)
	}
}

func TestInferNode_NVMRC(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".nvmrc", "20.10.0\n")
	reqs, err := InferNode(dir)
	if err != nil {
		t.Fatalf("InferNode: %v", err)
	}
	want := []NodeRequirement{{Source: ".nvmrc", Constraint: "20.10.0", IsExact: true}}
	if !reflect.DeepEqual(reqs, want) {
		t.Errorf("got %+v, want %+v", reqs, want)
	}
}

func TestInferNode_ToolVersions(t *testing.T) {
	cases := []struct {
		name, body, wantConstraint string
		want                       bool
	}{
		{"nodejs entry", "nodejs 20.10.0\n", "20.10.0", true},
		{"node entry alias", "node 18.19.0\n", "18.19.0", true},
		{"with comment", "# comment\nnodejs 20.10.0 # inline\n", "20.10.0", true},
		{"python only", "python 3.11.5\n", "", false},
		{"empty file", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, ".tool-versions", c.body)
			reqs, err := InferNode(dir)
			if err != nil {
				t.Fatalf("InferNode: %v", err)
			}
			if c.want {
				if len(reqs) != 1 || reqs[0].Constraint != c.wantConstraint {
					t.Errorf("got %+v, want one .tool-versions req with constraint %q", reqs, c.wantConstraint)
				}
			} else {
				if len(reqs) != 0 {
					t.Errorf("expected no requirements; got %+v", reqs)
				}
			}
		})
	}
}

func TestInferNode_MiseTOML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "mise.toml", `[tools]
node = "20.10.0"
python = "3.11.5"
`)
	reqs, err := InferNode(dir)
	if err != nil {
		t.Fatalf("InferNode: %v", err)
	}
	if len(reqs) != 1 || reqs[0].Source != "mise.toml" || reqs[0].Constraint != "20.10.0" {
		t.Errorf("got %+v; want mise.toml node 20.10.0", reqs)
	}
}

func TestInferNode_MiseTOMLObjectForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".mise.toml", `[tools]
node = { version = "20.10.0" }
`)
	reqs, err := InferNode(dir)
	if err != nil {
		t.Fatalf("InferNode: %v", err)
	}
	if len(reqs) != 1 || reqs[0].Constraint != "20.10.0" {
		t.Errorf("got %+v; want object-form node 20.10.0", reqs)
	}
}

func TestInferNode_PackageJSONEngines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{
  "name": "x",
  "engines": { "node": "^20.0.0" }
}`)
	reqs, err := InferNode(dir)
	if err != nil {
		t.Fatalf("InferNode: %v", err)
	}
	if len(reqs) != 1 || reqs[0].Source != "package.json#engines.node" || reqs[0].Constraint != "^20.0.0" || reqs[0].IsExact {
		t.Errorf("got %+v; want package.json engines.node ^20.0.0 (IsExact=false)", reqs)
	}
}

func TestInferNode_PackageJSONNoEnginesField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "x"}`)
	reqs, err := InferNode(dir)
	if err != nil {
		t.Fatalf("InferNode: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("expected no requirements when engines.node is absent; got %+v", reqs)
	}
}

func TestInferNode_MultipleSourcesAllReturned(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".nvmrc", "20.10.0\n")
	writeFile(t, dir, "package.json", `{"engines":{"node":"^20.0.0"}}`)
	reqs, err := InferNode(dir)
	if err != nil {
		t.Fatalf("InferNode: %v", err)
	}
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requirements; got %d: %+v", len(reqs), reqs)
	}
	if reqs[0].Source != ".nvmrc" {
		t.Errorf(".nvmrc must come first; got %q", reqs[0].Source)
	}
	if reqs[1].Source != "package.json#engines.node" {
		t.Errorf("package.json must come last; got %q", reqs[1].Source)
	}
}

func TestInferNode_MalformedPackageJSONIsError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{ this is not valid json")
	_, err := InferNode(dir)
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestNodeRequirement_AsConstraint(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"20", "^20.0.0"},
		{"v20", "^20.0.0"},
		{"20.10", "~20.10.0"},
		{"20.10.0", "20.10.0"},
		{"v20.10.0", "20.10.0"},
		{"^20.0.0", "^20.0.0"},
		{"~20.10.0", "~20.10.0"},
		{">=18", ">=18"},
		{"20.x", "20.x"},
		{"  20.10.0  ", "20.10.0"},
	}
	for _, c := range cases {
		r := NodeRequirement{Constraint: c.in}
		if got := r.AsConstraint(); got != c.want {
			t.Errorf("AsConstraint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
