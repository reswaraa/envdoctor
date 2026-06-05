// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package recipes

import (
	"strings"
	"testing"

	"github.com/reswaraa/envdoctor/internal/system"
)

// mkFacts constructs a *Facts safe for HasTool calls (Facts.HasTool
// lazy-inits the cache under its mutex). Matcher tests that need a
// specific HasTool outcome use "go" (always present in dev/CI) for
// presence and a synthetic name for absence.
func mkFacts(os, arch, distro string) *system.Facts {
	return &system.Facts{OS: os, Arch: arch, Distro: distro}
}

func TestSelectFix_FirstMatchWins(t *testing.T) {
	r := Recipe{
		ID:    "x",
		Probe: "p",
		Fixes: []Fix{
			{ID: "linux-only", When: Match{OS: "linux"}, Class: ClassSafe, Command: "echo linux"},
			{ID: "darwin-only", When: Match{OS: "darwin"}, Class: ClassSafe, Command: "echo darwin"},
		},
	}
	fix, cmd, err := SelectFix(r, mkFacts("darwin", "arm64", ""), nil)
	if err != nil {
		t.Fatalf("SelectFix: %v", err)
	}
	if fix.ID != "darwin-only" || cmd != "echo darwin" {
		t.Errorf("got fix=%q cmd=%q; want darwin-only / echo darwin", fix.ID, cmd)
	}
}

func TestSelectFix_FallbackOnlyWhenNoPrimaryMatches(t *testing.T) {
	r := Recipe{
		ID:    "x",
		Probe: "p",
		Fixes: []Fix{
			{ID: "primary", When: Match{OS: "linux"}, Class: ClassSafe, Command: "primary"},
			{ID: "fallback-mac", When: Match{OS: "darwin"}, Class: ClassShared, Command: "fb-mac", Fallback: true},
		},
	}
	fix, _, err := SelectFix(r, mkFacts("darwin", "arm64", ""), nil)
	if err != nil {
		t.Fatalf("SelectFix: %v", err)
	}
	if fix.ID != "fallback-mac" {
		t.Errorf("got %q; want fallback-mac", fix.ID)
	}
}

func TestSelectFix_PrimaryBeatsFallback(t *testing.T) {
	r := Recipe{
		ID:    "x",
		Probe: "p",
		Fixes: []Fix{
			{ID: "fb", When: Match{}, Class: ClassShared, Command: "fb", Fallback: true},
			{ID: "primary", When: Match{OS: "darwin"}, Class: ClassSafe, Command: "primary"},
		},
	}
	fix, _, err := SelectFix(r, mkFacts("darwin", "arm64", ""), nil)
	if err != nil {
		t.Fatalf("SelectFix: %v", err)
	}
	if fix.ID != "primary" {
		t.Errorf("got %q; want primary (primary must beat fallback even when fallback is declared first)", fix.ID)
	}
}

func TestSelectFix_NoMatchReturnsZero(t *testing.T) {
	r := Recipe{
		ID:    "x",
		Probe: "p",
		Fixes: []Fix{
			{ID: "linux-only", When: Match{OS: "linux"}, Class: ClassSafe, Command: "x"},
		},
	}
	fix, cmd, err := SelectFix(r, mkFacts("darwin", "arm64", ""), nil)
	if err != nil {
		t.Fatalf("SelectFix: %v", err)
	}
	if fix.ID != "" || cmd != "" {
		t.Errorf("expected zero result; got fix=%q cmd=%q", fix.ID, cmd)
	}
}

func TestSelectFix_EmptyWhenAlwaysMatches(t *testing.T) {
	r := Recipe{
		ID:    "x",
		Probe: "p",
		Fixes: []Fix{
			{ID: "any", When: Match{}, Class: ClassSafe, Command: "any"},
		},
	}
	fix, _, err := SelectFix(r, mkFacts("linux", "amd64", "ubuntu"), nil)
	if err != nil {
		t.Fatalf("SelectFix: %v", err)
	}
	if fix.ID != "any" {
		t.Errorf("empty When must always match; got %q", fix.ID)
	}
}

func TestSelectFix_TemplateExpansion(t *testing.T) {
	r := Recipe{
		ID:    "x",
		Probe: "p",
		Fixes: []Fix{
			{ID: "f", When: Match{}, Class: ClassSafe, Command: "install node@{{.Required}}"},
		},
	}
	_, cmd, err := SelectFix(r, mkFacts("darwin", "arm64", ""), map[string]any{"Required": "20.10.0"})
	if err != nil {
		t.Fatalf("SelectFix: %v", err)
	}
	if cmd != "install node@20.10.0" {
		t.Errorf("got %q; want install node@20.10.0", cmd)
	}
}

func TestSelectFix_TemplateMissingParamIsError(t *testing.T) {
	r := Recipe{
		ID:    "x",
		Probe: "p",
		Fixes: []Fix{
			{ID: "f", When: Match{}, Class: ClassSafe, Command: "install {{.Required}}"},
		},
	}
	_, cmd, err := SelectFix(r, mkFacts("darwin", "arm64", ""), map[string]any{})
	if err == nil {
		t.Fatalf("expected error for missing template param; got cmd=%q", cmd)
	}
	if !strings.Contains(err.Error(), "Required") {
		t.Errorf("error should mention the missing key; got: %v", err)
	}
}

func TestSelectFix_TemplateBadSyntaxIsError(t *testing.T) {
	r := Recipe{
		ID:    "x",
		Probe: "p",
		Fixes: []Fix{
			{ID: "f", When: Match{}, Class: ClassSafe, Command: "install {{.Required"},
		},
	}
	_, _, err := SelectFix(r, mkFacts("darwin", "arm64", ""), nil)
	if err == nil {
		t.Fatal("expected parse error for malformed template")
	}
}

// TestMatches_HasToolUsesFactsCache uses real exec.LookPath via the
// Facts.HasTool path; "go" must be present in any dev/CI environment
// and a synthetic name must be absent.
func TestMatches_HasToolUsesFactsCache(t *testing.T) {
	f := mkFacts("darwin", "arm64", "")

	if !matches(Match{HasTool: "go"}, f) {
		t.Error("expected match when has_tool: go (go must be on PATH in dev/CI)")
	}
	if matches(Match{HasTool: "definitely-not-a-real-binary-zzz"}, f) {
		t.Error("expected no match for synthetic missing tool")
	}
}

func TestMatches_AllFieldsMustMatch(t *testing.T) {
	f := mkFacts("linux", "amd64", "ubuntu")
	cases := []struct {
		name string
		m    Match
		want bool
	}{
		{"empty matches all", Match{}, true},
		{"os match", Match{OS: "linux"}, true},
		{"os mismatch", Match{OS: "darwin"}, false},
		{"arch match", Match{Arch: "amd64"}, true},
		{"arch mismatch", Match{Arch: "arm64"}, false},
		{"distro match", Match{Distro: "ubuntu"}, true},
		{"distro mismatch", Match{Distro: "alpine"}, false},
		{"all match", Match{OS: "linux", Arch: "amd64", Distro: "ubuntu"}, true},
		{"partial mismatch", Match{OS: "linux", Arch: "arm64", Distro: "ubuntu"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := matches(c.m, f); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
