// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/system"
)

const testDockerRecipes = `
id: docker-cli-missing
probe: docker-running
fixes:
  - id: brew-cask-docker
    class: shared
    when: { has_tool: brew }
    command: "brew install --cask docker"
`

const testDockerDaemonRecipe = `
id: docker-daemon-down
probe: docker-running
fixes:
  - id: open-docker-desktop
    class: safe
    when: { os: darwin }
    command: "open -a Docker"
`

func TestDockerRunning_ID(t *testing.T) {
	if got := DockerRunning(nil).ID(); got != "docker-running" {
		t.Errorf("got %q, want docker-running", got)
	}
}

func TestDockerRunning_AppliesTo(t *testing.T) {
	p := DockerRunning(nil)

	if p.AppliesTo(Input{RepoRoot: t.TempDir()}) {
		t.Error("empty repo must not apply")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !p.AppliesTo(Input{RepoRoot: dir}) {
		t.Error("repo with Dockerfile must apply")
	}
}

func TestDockerRunning_CLIMissing(t *testing.T) {
	// Drop PATH so HasTool("docker") returns false deterministically.
	t.Setenv("PATH", "")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := mkLibrary(t, testDockerRecipes)
	p := &dockerRunningProbe{lib: lib, dockerInfo: func(_ context.Context) error { return nil }}

	findings, err := p.Run(context.Background(), Input{RepoRoot: dir, System: &system.Facts{OS: "darwin"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings: got %d, want 1", len(findings))
	}
	f := findings[0]
	if !strings.Contains(f.Summary, "CLI not found") {
		t.Errorf("Summary should mention CLI; got %q", f.Summary)
	}
	if f.Category != output.CategoryDocker {
		t.Errorf("Category: got %q, want docker", f.Category)
	}
}

func TestDockerRunning_DaemonDown(t *testing.T) {
	// Make HasTool("docker") deterministically true by overriding PATH
	// to a temp dir containing only a stub `docker` executable. Stub is
	// never invoked — only LookPath touches it.
	facts := &system.Facts{OS: "darwin"}
	bin := t.TempDir()
	dockerFake := filepath.Join(bin, "docker")
	if err := os.WriteFile(dockerFake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := mkLibrary(t, testDockerDaemonRecipe)
	p := &dockerRunningProbe{
		lib: lib,
		dockerInfo: func(_ context.Context) error {
			return errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock")
		},
	}

	findings, err := p.Run(context.Background(), Input{RepoRoot: dir, System: facts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings: got %d, want 1", len(findings))
	}
	f := findings[0]
	if !strings.Contains(f.Summary, "daemon") {
		t.Errorf("Summary should mention daemon; got %q", f.Summary)
	}
	if !strings.Contains(f.Observed, "Cannot connect") {
		t.Errorf("Observed should surface docker info error; got %q", f.Observed)
	}
	if f.RecipeID != "open-docker-desktop" {
		t.Errorf("RecipeID: got %q, want open-docker-desktop", f.RecipeID)
	}
}

func TestDockerRunning_AllClear(t *testing.T) {
	bin := t.TempDir()
	dockerFake := filepath.Join(bin, "docker")
	if err := os.WriteFile(dockerFake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &dockerRunningProbe{lib: nil, dockerInfo: func(_ context.Context) error { return nil }}
	findings, err := p.Run(context.Background(), Input{RepoRoot: dir, System: &system.Facts{OS: "linux"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected no findings; got %+v", findings)
	}
}

func TestFirstNonEmptyLine(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{"\n\nfirst\nsecond", "first"},
		{"", ""},
		{"   \n  \nactual\n", "actual"},
		{strings.Repeat("x", 250), strings.Repeat("x", 200) + "..."},
	}
	for _, c := range cases {
		if got := firstNonEmptyLine(c.in); got != c.want {
			t.Errorf("firstNonEmptyLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
