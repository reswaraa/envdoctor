// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/reswaraa/envdoctor/internal/inference"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/recipes"
	"github.com/reswaraa/envdoctor/internal/semver"
	"github.com/reswaraa/envdoctor/internal/system"
)

const (
	nodeProbeID  = "node-version"
	nodeRecipeID = "node-version-mismatch"
	nodeDocURL   = "https://envdoctor.dev/probes/node-version"
)

// NodeVersion returns the Probe that compares the installed Node
// version against requirements inferred from .nvmrc, .node-version,
// .tool-versions, .mise.toml / mise.toml, package.json#engines.node.
func NodeVersion(lib *recipes.Library) Probe {
	return &nodeVersionProbe{lib: lib, detectVersion: realDetectNodeVersion}
}

type nodeVersionProbe struct {
	lib           *recipes.Library
	detectVersion func(context.Context) (string, error)
}

func (p *nodeVersionProbe) ID() string { return nodeProbeID }

func (p *nodeVersionProbe) AppliesTo(in Input) bool {
	reqs, err := inference.InferNode(in.RepoRoot)
	if err != nil {
		return true
	}
	return len(reqs) > 0
}

func (p *nodeVersionProbe) Run(ctx context.Context, in Input) ([]output.Finding, error) {
	reqs, err := inference.InferNode(in.RepoRoot)
	if err != nil {
		return nil, err
	}
	if len(reqs) == 0 {
		return nil, nil
	}
	installed, err := p.detectVersion(ctx)
	if err != nil {
		return p.evaluate(reqs, "", in.System), nil
	}
	return p.evaluate(reqs, installed, in.System), nil
}

// evaluate is the pure-function core: given inferred requirements, the
// detected installed version (empty string means "not installed"), and
// the system facts, return findings. Pure-function shape lets tests
// drive it without needing exec or a real PATH.
func (p *nodeVersionProbe) evaluate(reqs []inference.NodeRequirement, installed string, facts *system.Facts) []output.Finding {
	if len(reqs) == 0 {
		return nil
	}
	if installed == "" {
		return []output.Finding{p.notInstalledFinding(reqs, facts)}
	}

	var violations []inference.NodeRequirement
	for _, r := range reqs {
		ok, err := semver.Satisfies(installed, r.AsConstraint())
		if err != nil {
			continue
		}
		if !ok {
			violations = append(violations, r)
		}
	}
	if len(violations) == 0 {
		return nil
	}
	return []output.Finding{p.mismatchFinding(installed, violations, facts)}
}

func (p *nodeVersionProbe) mismatchFinding(installed string, violations []inference.NodeRequirement, facts *system.Facts) output.Finding {
	best := pickBestRequirement(violations)
	evidence := make([]string, 0, len(violations))
	for _, r := range violations {
		evidence = append(evidence, r.Source)
	}
	f := output.Finding{
		Probe:    nodeProbeID,
		Category: output.CategoryRuntime,
		Severity: output.SeverityError,
		Status:   output.StatusFail,
		Summary:  fmt.Sprintf("Node %s detected; repo requires %s", installed, best.Constraint),
		Observed: installed,
		Expected: best.Constraint,
		Evidence: evidence,
		DocURL:   nodeDocURL,
	}
	return p.applyRecipe(f, best, facts)
}

func (p *nodeVersionProbe) notInstalledFinding(reqs []inference.NodeRequirement, facts *system.Facts) output.Finding {
	best := pickBestRequirement(reqs)
	evidence := make([]string, 0, len(reqs))
	for _, r := range reqs {
		evidence = append(evidence, r.Source)
	}
	f := output.Finding{
		Probe:    nodeProbeID,
		Category: output.CategoryRuntime,
		Severity: output.SeverityError,
		Status:   output.StatusFail,
		Summary:  fmt.Sprintf("Node not installed; repo requires %s", best.Constraint),
		Observed: "(not installed)",
		Expected: best.Constraint,
		Evidence: evidence,
		DocURL:   nodeDocURL,
	}
	return p.applyRecipe(f, best, facts)
}

// pickBestRequirement returns the requirement most useful as the "expected"
// in a Finding: prefer exact-version sources (.nvmrc, .tool-versions) over
// constraint-range sources (package.json#engines) since exact versions
// translate directly to install commands.
func pickBestRequirement(reqs []inference.NodeRequirement) inference.NodeRequirement {
	for _, r := range reqs {
		if r.IsExact {
			return r
		}
	}
	return reqs[0]
}

func (p *nodeVersionProbe) applyRecipe(f output.Finding, req inference.NodeRequirement, facts *system.Facts) output.Finding {
	if p.lib == nil {
		return f
	}
	rec, ok := p.lib.Lookup(nodeRecipeID)
	if !ok {
		return f
	}
	target := strings.TrimPrefix(strings.TrimSpace(req.Constraint), "v")
	major, _ := semver.Major(req.Constraint)
	params := map[string]any{
		"Required":     target,
		"MajorVersion": fmt.Sprintf("%d", major),
	}
	fix, cmd, err := recipes.SelectFix(rec, facts, params)
	if err != nil || cmd == "" {
		return f
	}
	f.RecipeID = fix.ID
	f.RecipeClass = string(fix.Class)
	f.RecipeCommand = cmd
	return f
}

func realDetectNodeVersion(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "node", "--version").Output()
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(out))
	s = strings.TrimPrefix(s, "v")
	return s, nil
}
