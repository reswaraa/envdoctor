// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"reflect"
	"sort"
	"testing"

	"github.com/reswaraa/envdoctor/internal/config"
	"github.com/reswaraa/envdoctor/internal/recipes"
)

func TestBuiltinProbes_ContainsExpectedIDs(t *testing.T) {
	lib, err := recipes.DefaultLibrary()
	if err != nil {
		t.Fatalf("DefaultLibrary: %v", err)
	}
	ps := BuiltinProbes(lib, nil)
	if len(ps) != 9 {
		t.Fatalf("expected 9 probes; got %d", len(ps))
	}
	ids := make([]string, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, p.ID())
	}
	sort.Strings(ids)
	want := []string{
		"arch-mismatch", "docker-running", "env-required",
		"go-version", "node-version", "path-command",
		"port-free", "python-version", "ruby-version",
	}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("probe IDs: got %v, want %v", ids, want)
	}
}

func TestBuiltinProbes_NilLibAllowed(t *testing.T) {
	ps := BuiltinProbes(nil, nil)
	if len(ps) != 9 {
		t.Fatalf("expected 9 probes even with nil lib/cfg; got %d", len(ps))
	}
}

func TestBuiltinProbes_AppendsCustomWhenConfigHasChecks(t *testing.T) {
	cfg := &config.Config{
		SchemaVersion: 1,
		Checks: []config.Check{
			{Type: config.CheckCommandPresent, Command: "git"},
		},
	}
	ps := BuiltinProbes(nil, cfg)
	if len(ps) != 10 {
		t.Fatalf("expected 10 probes (9 builtins + custom); got %d", len(ps))
	}
	found := false
	for _, p := range ps {
		if p.ID() == "custom" {
			found = true
		}
	}
	if !found {
		t.Error("custom probe must be appended when config.Checks is non-empty")
	}
}

func TestBuiltinProbes_NoCustomWhenConfigHasNoChecks(t *testing.T) {
	cfg := &config.Config{SchemaVersion: 1}
	ps := BuiltinProbes(nil, cfg)
	if len(ps) != 9 {
		t.Errorf("expected 9 probes (no custom for empty checks); got %d", len(ps))
	}
}
