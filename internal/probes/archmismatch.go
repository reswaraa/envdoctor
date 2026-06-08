// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"context"
	"fmt"

	"github.com/reswaraa/envdoctor/internal/inference"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/recipes"
	"github.com/reswaraa/envdoctor/internal/semver"
	"github.com/reswaraa/envdoctor/internal/system"
)

const (
	archProbeID  = "arch-mismatch"
	archRecipeID = "native-arm64-missing"
	archDocURL   = "https://envdoctor.dev/probes/arch-mismatch"
)

// x86Issue describes a Node native package that lacked arm64 prebuilts
// before a known fixed version. The probe flags the dep when the
// system is arm64 and the installed version satisfies BadRange.
type x86Issue struct {
	BadRange     string
	FixedVersion string
}

// knownX86Issues is the curated set. Adding an entry also requires
// updating inference.nativeDepNames so the lockfile scan picks it up.
var knownX86Issues = map[string]x86Issue{
	// sharp added universal arm64 prebuilds in 0.33.0.
	"sharp": {BadRange: "<0.33.0", FixedVersion: "0.33.0"},
	// node-canvas (package name "canvas") shipped arm64 prebuilds in 2.11.0.
	"canvas": {BadRange: "<2.11.0", FixedVersion: "2.11.0"},
	// Cypress published native arm64 builds starting v13.
	"cypress": {BadRange: "<13.0.0", FixedVersion: "13.0.0"},
}

// ArchMismatch returns the Probe that warns Apple Silicon (and other
// linux/arm64) users when a lockfile pins a known x86-only version of
// a native Node package.
//
// AppliesTo only when system arch is arm64 AND the repo has a parseable
// lockfile. The probe does not detect Docker image arch mismatches in
// MVP (that needs `docker image inspect`); native-dep lockfile
// scanning hits ~80% of the actual user pain.
func ArchMismatch(lib *recipes.Library) Probe {
	return &archMismatchProbe{lib: lib}
}

type archMismatchProbe struct {
	lib *recipes.Library
}

func (p *archMismatchProbe) ID() string { return archProbeID }

func (p *archMismatchProbe) AppliesTo(in Input) bool {
	if in.System.Arch != "arm64" {
		return false
	}
	return inference.HasNodeLockfile(in.RepoRoot)
}

func (p *archMismatchProbe) Run(_ context.Context, in Input) ([]output.Finding, error) {
	deps, err := inference.InferNativeArchDeps(in.RepoRoot)
	if err != nil {
		return nil, err
	}
	if len(deps) == 0 {
		return nil, nil
	}
	return p.evaluate(deps, in.System), nil
}

// evaluate is the pure-function core. For each scanned dep, look up
// whether it's a known x86-only case and check the version range.
// Returns one Finding per matching dep.
func (p *archMismatchProbe) evaluate(deps []inference.NativeDep, facts *system.Facts) []output.Finding {
	var out []output.Finding
	for _, d := range deps {
		issue, ok := knownX86Issues[d.Name]
		if !ok {
			continue
		}
		bad, err := semver.Satisfies(d.Version, issue.BadRange)
		if err != nil || !bad {
			continue
		}
		out = append(out, p.findingFor(d, issue, facts))
	}
	return out
}

func (p *archMismatchProbe) findingFor(d inference.NativeDep, issue x86Issue, facts *system.Facts) output.Finding {
	f := output.Finding{
		Probe:    archProbeID,
		Category: output.CategoryArchitecture,
		Severity: output.SeverityWarning,
		Status:   output.StatusFail,
		Summary: fmt.Sprintf(
			"%s %s pre-dates arm64 prebuilds; install will rebuild from source or fail",
			d.Name, d.Version),
		Observed: d.Version,
		Expected: fmt.Sprintf(">=%s", issue.FixedVersion),
		Evidence: []string{d.Source},
		DocURL:   archDocURL,
	}
	return p.applyRecipe(f, d.Name, issue, facts)
}

func (p *archMismatchProbe) applyRecipe(f output.Finding, name string, issue x86Issue, facts *system.Facts) output.Finding {
	if p.lib == nil {
		return f
	}
	rec, ok := p.lib.Lookup(archRecipeID)
	if !ok {
		return f
	}
	params := map[string]any{
		"Name":         name,
		"FixedVersion": issue.FixedVersion,
	}
	fix, cmd, err := recipes.SelectFix(rec, facts, params)
	if err != nil || cmd == "" {
		return f
	}
	f.RecipeID = fix.ID
	f.RecipeCommand = cmd
	return f
}
