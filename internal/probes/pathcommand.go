// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"context"
	"fmt"

	"github.com/reswaraa/envdoctor/internal/inference"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/recipes"
	"github.com/reswaraa/envdoctor/internal/system"
)

const (
	pathProbeID  = "path-command"
	pathRecipeID = "path-command-missing"
	pathDocURL   = "https://reswaraa.github.io/envdoctor/probes/path-command"
)

// PathCommand returns the Probe that flags binaries referenced by
// Makefile / Procfile / docker-compose.yml that aren't on PATH.
// One Finding per missing command. The recipe library has install
// commands for a curated set of common tools; missing commands not in
// the set produce findings without recipes (exit code 2: envdoctor
// needs a new recipe).
func PathCommand(lib *recipes.Library) Probe {
	return &pathCommandProbe{lib: lib}
}

type pathCommandProbe struct {
	lib *recipes.Library
}

func (p *pathCommandProbe) ID() string { return pathProbeID }

func (p *pathCommandProbe) AppliesTo(in Input) bool {
	reqs, err := inference.InferCommands(in.RepoRoot)
	if err != nil {
		return true
	}
	return len(reqs) > 0
}

func (p *pathCommandProbe) Run(_ context.Context, in Input) ([]output.Finding, error) {
	reqs, err := inference.InferCommands(in.RepoRoot)
	if err != nil {
		return nil, err
	}
	if len(reqs) == 0 {
		return nil, nil
	}
	return p.evaluate(reqs, in.System), nil
}

func (p *pathCommandProbe) evaluate(reqs []inference.CommandRequirement, facts *system.Facts) []output.Finding {
	var out []output.Finding
	for _, r := range reqs {
		if facts.HasTool(r.Command) {
			continue
		}
		out = append(out, p.findingFor(r, facts))
	}
	return out
}

// commandPackages maps command names to per-package-manager install
// package names. Only commands listed here get a recipe attached;
// missing commands not in this map still produce a Finding (the user
// sees "X not on PATH") but no fix command — exit code 2 signals
// "envdoctor needs a new recipe", inviting a PR.
var commandPackages = map[string]struct{ Brew, Apt string }{
	"make":      {Brew: "make", Apt: "build-essential"},
	"psql":      {Brew: "libpq", Apt: "postgresql-client"},
	"protoc":    {Brew: "protobuf", Apt: "protobuf-compiler"},
	"git-lfs":   {Brew: "git-lfs", Apt: "git-lfs"},
	"jq":        {Brew: "jq", Apt: "jq"},
	"yq":        {Brew: "yq", Apt: "yq"},
	"redis-cli": {Brew: "redis", Apt: "redis-tools"},
}

func (p *pathCommandProbe) findingFor(r inference.CommandRequirement, facts *system.Facts) output.Finding {
	f := output.Finding{
		Probe:    pathProbeID,
		Category: output.CategoryPath,
		Severity: output.SeverityError,
		Status:   output.StatusFail,
		Summary:  fmt.Sprintf("Command %q referenced by %s is not on PATH", r.Command, r.Source),
		Observed: "(missing)",
		Expected: fmt.Sprintf("%q on PATH", r.Command),
		Evidence: []string{r.Source},
		DocURL:   pathDocURL,
	}
	return p.applyRecipe(f, r.Command, facts)
}

func (p *pathCommandProbe) applyRecipe(f output.Finding, cmd string, facts *system.Facts) output.Finding {
	if p.lib == nil {
		return f
	}
	pkgs, ok := commandPackages[cmd]
	if !ok {
		return f
	}
	rec, ok := p.lib.Lookup(pathRecipeID)
	if !ok {
		return f
	}
	params := map[string]any{
		"Command":     cmd,
		"BrewPackage": pkgs.Brew,
		"AptPackage":  pkgs.Apt,
	}
	fix, cmdStr, err := recipes.SelectFix(rec, facts, params)
	if err != nil || cmdStr == "" {
		return f
	}
	f.RecipeID = fix.ID
	f.RecipeClass = string(fix.Class)
	f.RecipeCommand = cmdStr
	return f
}
