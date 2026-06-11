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
	goProbeID  = "go-version"
	goRecipeID = "go-version-mismatch"
	goDocURL   = "https://reswaraa.github.io/envdoctor/probes/go-version"
)

// GoVersion mirrors NodeVersion for Go. Inference reads go.mod's `go`
// directive only. go.mod's directive is documented as a minimum, so
// the probe compares installed against ">=X.Y.Z" rather than an exact
// pin.
func GoVersion(lib *recipes.Library) Probe {
	return &goVersionProbe{lib: lib, detectVersion: realDetectGoVersion}
}

type goVersionProbe struct {
	lib           *recipes.Library
	detectVersion func(context.Context) (string, error)
}

func (p *goVersionProbe) ID() string { return goProbeID }

func (p *goVersionProbe) AppliesTo(in Input) bool {
	reqs, err := inference.InferGo(in.RepoRoot)
	if err != nil {
		return true
	}
	return len(reqs) > 0
}

func (p *goVersionProbe) Run(ctx context.Context, in Input) ([]output.Finding, error) {
	reqs, err := inference.InferGo(in.RepoRoot)
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

func (p *goVersionProbe) evaluate(reqs []inference.GoRequirement, installed string, facts *system.Facts) []output.Finding {
	if len(reqs) == 0 {
		return nil
	}
	req := reqs[0]
	if installed == "" {
		return []output.Finding{p.findingFor(req, "(not installed)", facts)}
	}
	ok, err := semver.Satisfies(installed, req.AsConstraint())
	if err != nil || ok {
		return nil
	}
	return []output.Finding{p.findingFor(req, installed, facts)}
}

func (p *goVersionProbe) findingFor(req inference.GoRequirement, observed string, facts *system.Facts) output.Finding {
	f := output.Finding{
		Probe:    goProbeID,
		Category: output.CategoryRuntime,
		Severity: output.SeverityError,
		Status:   output.StatusFail,
		Summary:  fmt.Sprintf("Go %s detected; repo requires >=%s", observed, req.Constraint),
		Observed: observed,
		Expected: ">=" + req.Constraint,
		Evidence: []string{req.Source},
		DocURL:   goDocURL,
	}
	return p.applyRecipe(f, req, facts)
}

func (p *goVersionProbe) applyRecipe(f output.Finding, req inference.GoRequirement, facts *system.Facts) output.Finding {
	if p.lib == nil {
		return f
	}
	rec, ok := p.lib.Lookup(goRecipeID)
	if !ok {
		return f
	}
	major, _ := semver.Major(req.Constraint)
	params := map[string]any{
		"Required":     strings.TrimSpace(req.Constraint),
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

func realDetectGoVersion(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "go", "version").Output()
	if err != nil {
		return "", err
	}
	// "go version go1.21.5 darwin/arm64" → "1.21.5"
	fields := strings.Fields(string(out))
	for _, f := range fields {
		if strings.HasPrefix(f, "go") && len(f) > 2 && (f[2] >= '0' && f[2] <= '9') {
			return strings.TrimPrefix(f, "go"), nil
		}
	}
	return "", fmt.Errorf("could not parse go version output: %s", string(out))
}
