// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/reswaraa/envdoctor/internal/inference"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/recipes"
	"github.com/reswaraa/envdoctor/internal/system"
)

const (
	envProbeID  = "env-required"
	envRecipeID = "env-missing"
	envDocURL   = "https://envdoctor.dev/probes/env-required"
)

// EnvRequired returns the Probe that diffs required env vars (from
// .env.example and docker-compose.yml) against present env vars (from
// the repo's .env file plus the process environment).
//
// Only key NAMES are ever recorded in the Finding; values are not read
// or emitted by this probe. This is a structural-redaction guarantee
// for the Phase 5 bundle: a buggy probe author cannot accidentally
// leak an env value here because values are not touched.
func EnvRequired(lib *recipes.Library) Probe {
	return &envRequiredProbe{
		lib:        lib,
		readDotenv: func(path string) ([]string, error) { return inference.ReadEnvKeys(path) },
		processEnv: os.Environ,
	}
}

type envRequiredProbe struct {
	lib        *recipes.Library
	readDotenv func(path string) ([]string, error)
	processEnv func() []string
}

func (p *envRequiredProbe) ID() string { return envProbeID }

func (p *envRequiredProbe) AppliesTo(in Input) bool {
	reqs, err := inference.InferEnv(in.RepoRoot)
	if err != nil {
		return true
	}
	return len(reqs) > 0
}

func (p *envRequiredProbe) Run(_ context.Context, in Input) ([]output.Finding, error) {
	reqs, err := inference.InferEnv(in.RepoRoot)
	if err != nil {
		return nil, err
	}
	if len(reqs) == 0 {
		return nil, nil
	}
	present, err := p.collectPresent(in.RepoRoot)
	if err != nil {
		return nil, err
	}
	return p.evaluate(reqs, present, in.System), nil
}

// collectPresent returns the set of env keys defined in either the
// repo's .env file or the process environment.
func (p *envRequiredProbe) collectPresent(repoRoot string) (map[string]bool, error) {
	out := map[string]bool{}
	keys, err := p.readDotenv(filepath.Join(repoRoot, ".env"))
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		out[k] = true
	}
	for _, e := range p.processEnv() {
		if i := strings.Index(e, "="); i > 0 {
			out[e[:i]] = true
		}
	}
	return out, nil
}

// evaluate is the pure-function core. Given requirements, the set of
// present env keys, and the system facts, return findings.
func (p *envRequiredProbe) evaluate(reqs []inference.EnvRequirement, present map[string]bool, facts *system.Facts) []output.Finding {
	missing := make([]inference.EnvRequirement, 0)
	seen := map[string]bool{}
	for _, r := range reqs {
		if present[r.Key] {
			continue
		}
		if seen[r.Key] {
			continue
		}
		seen[r.Key] = true
		missing = append(missing, r)
	}
	if len(missing) == 0 {
		return nil
	}

	keys := make([]string, 0, len(missing))
	sources := map[string]bool{}
	for _, m := range missing {
		keys = append(keys, m.Key)
		sources[m.Source] = true
	}
	sort.Strings(keys)
	evidence := make([]string, 0, len(sources))
	for s := range sources {
		evidence = append(evidence, s)
	}
	sort.Strings(evidence)

	summary := fmt.Sprintf("%d required env var%s missing: %s",
		len(keys), pluralS(len(keys)), strings.Join(keys, ", "))

	f := output.Finding{
		Probe:    envProbeID,
		Category: output.CategoryEnvironment,
		Severity: output.SeverityError,
		Status:   output.StatusFail,
		Summary:  summary,
		Observed: "(missing)",
		Expected: strings.Join(keys, ", "),
		Evidence: evidence,
		DocURL:   envDocURL,
	}
	return []output.Finding{p.applyRecipe(f, facts)}
}

func (p *envRequiredProbe) applyRecipe(f output.Finding, facts *system.Facts) output.Finding {
	if p.lib == nil {
		return f
	}
	rec, ok := p.lib.Lookup(envRecipeID)
	if !ok {
		return f
	}
	fix, cmd, err := recipes.SelectFix(rec, facts, nil)
	if err != nil || cmd == "" {
		return f
	}
	f.RecipeID = fix.ID
	f.RecipeCommand = cmd
	return f
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
