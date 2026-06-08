// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"reflect"
	"testing"
)

func TestPythonRequirement_AsConstraint(t *testing.T) {
	cases := []struct{ in, want string }{
		{"3", "^3.0.0"},
		{"3.11", "~3.11.0"},
		{"3.11.5", "3.11.5"},
		{">=3.10", ">=3.10"},
		{"^3.10", "^3.10"},
	}
	for _, c := range cases {
		got := PythonRequirement{Constraint: c.in}.AsConstraint()
		if got != c.want {
			t.Errorf("AsConstraint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInferPython_AllSources(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".python-version", "3.11.5\n")
	writeFile(t, dir, ".tool-versions", "python 3.11.7\nnodejs 20.10.0\n")
	writeFile(t, dir, "mise.toml", "[tools]\npython = \"3.11.6\"\n")
	writeFile(t, dir, "pyproject.toml", `[project]
requires-python = ">=3.10"
`)
	reqs, err := InferPython(dir)
	if err != nil {
		t.Fatalf("InferPython: %v", err)
	}
	want := []PythonRequirement{
		{Source: ".python-version", Constraint: "3.11.5", IsExact: true},
		{Source: ".tool-versions", Constraint: "3.11.7", IsExact: true},
		{Source: "mise.toml", Constraint: "3.11.6", IsExact: true},
		{Source: "pyproject.toml#project.requires-python", Constraint: ">=3.10", IsExact: false},
	}
	if !reflect.DeepEqual(reqs, want) {
		t.Errorf("got %+v\nwant %+v", reqs, want)
	}
}

func TestInferPython_PoetryDependency(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", `[tool.poetry.dependencies]
python = "^3.10"
`)
	reqs, err := InferPython(dir)
	if err != nil {
		t.Fatalf("InferPython: %v", err)
	}
	if len(reqs) != 1 || reqs[0].Source != "pyproject.toml#tool.poetry.dependencies.python" || reqs[0].Constraint != "^3.10" {
		t.Errorf("got %+v; want Poetry python constraint ^3.10", reqs)
	}
}

func TestInferPython_EmptyRepo(t *testing.T) {
	reqs, err := InferPython(t.TempDir())
	if err != nil {
		t.Fatalf("InferPython: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("expected 0; got %+v", reqs)
	}
}
