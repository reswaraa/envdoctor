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
	pythonProbeID  = "python-version"
	pythonRecipeID = "python-version-mismatch"
	pythonDocURL   = "https://envdoctor.dev/probes/python-version"
)

// PythonVersion mirrors NodeVersion for Python: inference from
// .python-version, .tool-versions, mise.toml, pyproject.toml.
func PythonVersion(lib *recipes.Library) Probe {
	return &pythonVersionProbe{lib: lib, detectVersion: realDetectPythonVersion}
}

type pythonVersionProbe struct {
	lib           *recipes.Library
	detectVersion func(context.Context) (string, error)
}

func (p *pythonVersionProbe) ID() string { return pythonProbeID }

func (p *pythonVersionProbe) AppliesTo(in Input) bool {
	reqs, err := inference.InferPython(in.RepoRoot)
	if err != nil {
		return true
	}
	return len(reqs) > 0
}

func (p *pythonVersionProbe) Run(ctx context.Context, in Input) ([]output.Finding, error) {
	reqs, err := inference.InferPython(in.RepoRoot)
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

func (p *pythonVersionProbe) evaluate(reqs []inference.PythonRequirement, installed string, facts *system.Facts) []output.Finding {
	if len(reqs) == 0 {
		return nil
	}
	if installed == "" {
		return []output.Finding{p.findingFor(reqs, "(not installed)", reqs, facts, true)}
	}
	var violations []inference.PythonRequirement
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

func (p *pythonVersionProbe) findingFor(violations []inference.PythonRequirement, observed string, allReqs []inference.PythonRequirement, facts *system.Facts, notInstalled bool) output.Finding {
	best := pickExactPython(violations)
	summary := fmt.Sprintf("Python %s detected; repo requires %s", observed, best.Constraint)
	if notInstalled {
		summary = fmt.Sprintf("Python not installed; repo requires %s", best.Constraint)
	}
	evidence := make([]string, 0, len(allReqs))
	for _, r := range allReqs {
		evidence = append(evidence, r.Source)
	}
	f := output.Finding{
		Probe:    pythonProbeID,
		Category: output.CategoryRuntime,
		Severity: output.SeverityError,
		Status:   output.StatusFail,
		Summary:  summary,
		Observed: observed,
		Expected: best.Constraint,
		Evidence: evidence,
		DocURL:   pythonDocURL,
	}
	return p.applyRecipe(f, best, facts)
}

func pickExactPython(reqs []inference.PythonRequirement) inference.PythonRequirement {
	for _, r := range reqs {
		if r.IsExact {
			return r
		}
	}
	return reqs[0]
}

func (p *pythonVersionProbe) applyRecipe(f output.Finding, req inference.PythonRequirement, facts *system.Facts) output.Finding {
	if p.lib == nil {
		return f
	}
	rec, ok := p.lib.Lookup(pythonRecipeID)
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

func realDetectPythonVersion(ctx context.Context) (string, error) {
	for _, bin := range []string{"python3", "python"} {
		out, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput()
		if err != nil {
			continue
		}
		s := strings.TrimSpace(string(out))
		s = strings.TrimPrefix(s, "Python ")
		return s, nil
	}
	return "", fmt.Errorf("python not found")
}
