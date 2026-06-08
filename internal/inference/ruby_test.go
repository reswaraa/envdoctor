// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"reflect"
	"testing"
)

func TestRubyRequirement_AsConstraint(t *testing.T) {
	cases := []struct{ in, want string }{
		{"3.2.2", "3.2.2"},
		{"3.2", "~3.2.0"},
		{"3", "^3.0.0"},
		// RubyGems ~> semantics: 2-segment broadens to minor+patch
		// (caret), 3-segment locks to patch only (tilde).
		{"~> 3.2", "^3.2.0"},
		{"~> 3.2.0", "~3.2.0"},
		{"~> 3.2.5", "~3.2.5"},
		{">=3.0", ">=3.0"},
	}
	for _, c := range cases {
		got := RubyRequirement{Constraint: c.in}.AsConstraint()
		if got != c.want {
			t.Errorf("AsConstraint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInferRuby_AllSources(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".ruby-version", "3.2.2\n")
	writeFile(t, dir, ".tool-versions", "ruby 3.2.3\n")
	writeFile(t, dir, "Gemfile", `source 'https://rubygems.org'
ruby '3.2.4'
`)
	reqs, err := InferRuby(dir)
	if err != nil {
		t.Fatalf("InferRuby: %v", err)
	}
	want := []RubyRequirement{
		{Source: ".ruby-version", Constraint: "3.2.2", IsExact: true},
		{Source: ".tool-versions", Constraint: "3.2.3", IsExact: true},
		{Source: "Gemfile#ruby", Constraint: "3.2.4", IsExact: false},
	}
	if !reflect.DeepEqual(reqs, want) {
		t.Errorf("got %+v\nwant %+v", reqs, want)
	}
}

func TestInferRuby_GemfileWithTilde(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Gemfile", `ruby "~> 3.2"`)
	reqs, err := InferRuby(dir)
	if err != nil {
		t.Fatalf("InferRuby: %v", err)
	}
	if len(reqs) != 1 || reqs[0].Constraint != "~> 3.2" {
		t.Errorf("got %+v; want one Gemfile entry with '~> 3.2'", reqs)
	}
}

func TestInferRuby_EmptyRepo(t *testing.T) {
	reqs, err := InferRuby(t.TempDir())
	if err != nil {
		t.Fatalf("InferRuby: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("expected 0; got %+v", reqs)
	}
}
