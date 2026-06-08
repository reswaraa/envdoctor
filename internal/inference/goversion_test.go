// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"testing"
)

func TestGoRequirement_AsConstraint(t *testing.T) {
	cases := []struct{ in, want string }{
		{"1.21", ">=1.21.0"},
		{"1.21.5", ">=1.21.5"},
		{"1", ">=1.0.0"},
		{">=1.21", ">=1.21"},
	}
	for _, c := range cases {
		got := GoRequirement{Constraint: c.in}.AsConstraint()
		if got != c.want {
			t.Errorf("AsConstraint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInferGo(t *testing.T) {
	cases := []struct {
		name, body, wantConstraint string
		want                       bool
	}{
		{"basic", "module x\n\ngo 1.21\n", "1.21", true},
		{"patch", "module x\n\ngo 1.21.5\n", "1.21.5", true},
		{"with comment", "module x\n\ngo 1.21 // toolchain hint\n", "1.21", true},
		{"no go line", "module x\n", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "go.mod", c.body)
			reqs, err := InferGo(dir)
			if err != nil {
				t.Fatalf("InferGo: %v", err)
			}
			if c.want {
				if len(reqs) != 1 || reqs[0].Constraint != c.wantConstraint {
					t.Errorf("got %+v; want one req with %q", reqs, c.wantConstraint)
				}
			} else if len(reqs) != 0 {
				t.Errorf("expected 0; got %+v", reqs)
			}
		})
	}
}

func TestInferGo_EmptyRepo(t *testing.T) {
	reqs, err := InferGo(t.TempDir())
	if err != nil {
		t.Fatalf("InferGo: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("expected 0; got %+v", reqs)
	}
}
