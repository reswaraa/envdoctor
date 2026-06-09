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
	rubyProbeID  = "ruby-version"
	rubyRecipeID = "ruby-version-mismatch"
	rubyDocURL   = "https://envdoctor.dev/probes/ruby-version"
)

// RubyVersion mirrors NodeVersion for Ruby. Inference from
// .ruby-version, .tool-versions, mise.toml, Gemfile's `ruby '...'`.
func RubyVersion(lib *recipes.Library) Probe {
	return &rubyVersionProbe{lib: lib, detectVersion: realDetectRubyVersion}
}

type rubyVersionProbe struct {
	lib           *recipes.Library
	detectVersion func(context.Context) (string, error)
}

func (p *rubyVersionProbe) ID() string { return rubyProbeID }

func (p *rubyVersionProbe) AppliesTo(in Input) bool {
	reqs, err := inference.InferRuby(in.RepoRoot)
	if err != nil {
		return true
	}
	return len(reqs) > 0
}

func (p *rubyVersionProbe) Run(ctx context.Context, in Input) ([]output.Finding, error) {
	reqs, err := inference.InferRuby(in.RepoRoot)
	if err != nil {
		return nil, err
	}
	if len(reqs) == 0 {
		return nil, nil
	}
	installed, err := p.detectVersion(ctx)
	if err != nil {
		installed = ""
	}
	return p.evaluate(reqs, installed, in.System), nil
}

func (p *rubyVersionProbe) evaluate(reqs []inference.RubyRequirement, installed string, facts *system.Facts) []output.Finding {
	if len(reqs) == 0 {
		return nil
	}
	if installed == "" {
		return []output.Finding{p.findingFor(reqs, "(not installed)", reqs, facts, true)}
	}
	var violations []inference.RubyRequirement
	for _, r := range reqs {
		ok, err := semver.Satisfies(installed, r.AsConstraint())
		if err != nil || ok {
			continue
		}
		violations = append(violations, r)
	}
	if len(violations) == 0 {
		return nil
	}
	return []output.Finding{p.findingFor(violations, installed, reqs, facts, false)}
}

func (p *rubyVersionProbe) findingFor(violations []inference.RubyRequirement, observed string, allReqs []inference.RubyRequirement, facts *system.Facts, notInstalled bool) output.Finding {
	best := pickExactRuby(violations)
	summary := fmt.Sprintf("Ruby %s detected; repo requires %s", observed, best.Constraint)
	if notInstalled {
		summary = fmt.Sprintf("Ruby not installed; repo requires %s", best.Constraint)
	}
	evidence := make([]string, 0, len(allReqs))
	for _, r := range allReqs {
		evidence = append(evidence, r.Source)
	}
	f := output.Finding{
		Probe:    rubyProbeID,
		Category: output.CategoryRuntime,
		Severity: output.SeverityError,
		Status:   output.StatusFail,
		Summary:  summary,
		Observed: observed,
		Expected: best.Constraint,
		Evidence: evidence,
		DocURL:   rubyDocURL,
	}
	return p.applyRecipe(f, best, facts)
}

func pickExactRuby(reqs []inference.RubyRequirement) inference.RubyRequirement {
	for _, r := range reqs {
		if r.IsExact {
			return r
		}
	}
	return reqs[0]
}

func (p *rubyVersionProbe) applyRecipe(f output.Finding, req inference.RubyRequirement, facts *system.Facts) output.Finding {
	if p.lib == nil {
		return f
	}
	rec, ok := p.lib.Lookup(rubyRecipeID)
	if !ok {
		return f
	}
	target := strings.TrimSpace(req.Constraint)
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

func realDetectRubyVersion(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "ruby", "--version").Output()
	if err != nil {
		return "", err
	}
	// "ruby 3.2.2 (2023-03-30 revision e51014f9c0) [arm64-darwin22]"
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return "", fmt.Errorf("could not parse ruby version output: %s", string(out))
	}
	return fields[1], nil
}
