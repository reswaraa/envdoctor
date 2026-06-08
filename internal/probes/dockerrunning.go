// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/reswaraa/envdoctor/internal/inference"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/recipes"
	"github.com/reswaraa/envdoctor/internal/system"
)

const (
	dockerProbeID          = "docker-running"
	dockerCLIMissingRecipe = "docker-cli-missing"
	dockerDaemonDownRecipe = "docker-daemon-down"
	dockerDocURL           = "https://envdoctor.dev/probes/docker-running"
)

// DockerRunning returns the Probe that checks the docker CLI is on
// PATH and the daemon answers. Two distinct finding shapes:
//
//	docker CLI missing  → docker-cli-missing recipe
//	docker daemon down  → docker-daemon-down recipe
//
// AppliesTo when the repo has a Dockerfile or any compose-family file.
func DockerRunning(lib *recipes.Library) Probe {
	return &dockerRunningProbe{lib: lib, dockerInfo: realDockerInfo}
}

type dockerRunningProbe struct {
	lib        *recipes.Library
	dockerInfo func(ctx context.Context) error
}

func (p *dockerRunningProbe) ID() string { return dockerProbeID }

func (p *dockerRunningProbe) AppliesTo(in Input) bool {
	return inference.HasDockerSignals(in.RepoRoot)
}

func (p *dockerRunningProbe) Run(ctx context.Context, in Input) ([]output.Finding, error) {
	if !in.System.HasTool("docker") {
		return []output.Finding{p.cliMissingFinding(in.System)}, nil
	}
	if err := p.dockerInfo(ctx); err != nil {
		return []output.Finding{p.daemonDownFinding(in.System, err)}, nil
	}
	return nil, nil
}

func (p *dockerRunningProbe) cliMissingFinding(facts *system.Facts) output.Finding {
	f := output.Finding{
		Probe:    dockerProbeID,
		Category: output.CategoryDocker,
		Severity: output.SeverityError,
		Status:   output.StatusFail,
		Summary:  "Docker CLI not found on PATH",
		Observed: "(missing)",
		Expected: "docker binary on PATH",
		DocURL:   dockerDocURL,
	}
	return p.applyRecipe(f, dockerCLIMissingRecipe, facts)
}

func (p *dockerRunningProbe) daemonDownFinding(facts *system.Facts, runErr error) output.Finding {
	observed := "docker info returned an error"
	if runErr != nil {
		if msg := firstNonEmptyLine(runErr.Error()); msg != "" {
			observed = msg
		}
	}
	f := output.Finding{
		Probe:    dockerProbeID,
		Category: output.CategoryDocker,
		Severity: output.SeverityError,
		Status:   output.StatusFail,
		Summary:  "Docker daemon is not responding",
		Observed: observed,
		Expected: "docker info to succeed",
		DocURL:   dockerDocURL,
	}
	return p.applyRecipe(f, dockerDaemonDownRecipe, facts)
}

func (p *dockerRunningProbe) applyRecipe(f output.Finding, recipeID string, facts *system.Facts) output.Finding {
	if p.lib == nil {
		return f
	}
	rec, ok := p.lib.Lookup(recipeID)
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

func realDockerInfo(parentCtx context.Context) error {
	ctx, cancel := context.WithTimeout(parentCtx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(buf.String())
		if line := firstNonEmptyLine(out); line != "" {
			return fmt.Errorf("%s", line)
		}
		return err
	}
	return nil
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 200 {
			return line[:200] + "..."
		}
		return line
	}
	return ""
}
