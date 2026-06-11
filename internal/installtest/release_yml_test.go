// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package installtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Pinned shapes for release.yml. Keep these structs at file scope
// so they can be shared between tests and helpers without
// cluttering each test body.

type releaseOn struct {
	Push struct {
		Tags []string `yaml:"tags"`
	} `yaml:"push"`
}

type releasePermissions struct {
	Contents string `yaml:"contents"`
}

type releaseStep struct {
	Name string `yaml:"name"`
	Uses string `yaml:"uses"`
	Run  string `yaml:"run"`
}

type releaseJob struct {
	RunsOn string        `yaml:"runs-on"`
	Steps  []releaseStep `yaml:"steps"`
}

type releaseWorkflow struct {
	Name        string                `yaml:"name"`
	On          releaseOn             `yaml:"on"`
	Permissions releasePermissions    `yaml:"permissions"`
	Jobs        map[string]releaseJob `yaml:"jobs"`
}

// TestReleaseWorkflow_ParsesAsYAML is the first line of defense
// against a syntactically broken release.yml — that would mean a
// tag push silently no-ops on GitHub Actions instead of cutting
// a release. Cheap to run, catches a real failure mode.
func TestReleaseWorkflow_ParsesAsYAML(t *testing.T) {
	path := findRepoFile(t, filepath.Join(".github", "workflows", "release.yml"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var parsed any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("release.yml is not valid YAML: %v", err)
	}
}

// TestReleaseWorkflow_PinsExpectedShape asserts the load-bearing
// pieces: tag-only trigger, contents:write permission (GoReleaser
// needs it to push the release), and the four expected steps
// (checkout, setup-go, test, goreleaser).
//
// Intentionally NOT asserting exact action versions — those move
// on their own cadence and tying tests to them would burn us on
// every dependabot bump.
func TestReleaseWorkflow_PinsExpectedShape(t *testing.T) {
	path := findRepoFile(t, filepath.Join(".github", "workflows", "release.yml"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var w releaseWorkflow
	if err := yaml.Unmarshal(raw, &w); err != nil {
		t.Fatalf("unmarshal release.yml: %v", err)
	}

	if w.Name != "release" {
		t.Errorf("name: got %q, want %q", w.Name, "release")
	}
	if len(w.On.Push.Tags) == 0 {
		t.Errorf("on.push.tags missing — release must trigger on tag push")
	}
	foundTag := false
	for _, tag := range w.On.Push.Tags {
		if tag == "v*.*.*" {
			foundTag = true
			break
		}
	}
	if !foundTag {
		t.Errorf("on.push.tags must include `v*.*.*`; got %v", w.On.Push.Tags)
	}
	if w.Permissions.Contents != "write" {
		t.Errorf("permissions.contents: got %q, want %q (GoReleaser needs it)", w.Permissions.Contents, "write")
	}

	job, ok := w.Jobs["goreleaser"]
	if !ok {
		var names []string
		for k := range w.Jobs {
			names = append(names, k)
		}
		t.Fatalf("missing job `goreleaser`; got jobs: %v", names)
	}
	if job.RunsOn != "ubuntu-latest" {
		t.Errorf("goreleaser.runs-on: got %q", job.RunsOn)
	}
	wantStepFragments := []string{
		"actions/checkout",
		"actions/setup-go",
		"go test",
		"goreleaser/goreleaser-action",
	}
	for _, want := range wantStepFragments {
		if !stepsContain(job.Steps, want) {
			t.Errorf("expected a step referencing %q; got steps:\n%+v", want, job.Steps)
		}
	}
}

func stepsContain(steps []releaseStep, fragment string) bool {
	for _, s := range steps {
		if strings.Contains(s.Uses, fragment) || strings.Contains(s.Run, fragment) {
			return true
		}
	}
	return false
}
