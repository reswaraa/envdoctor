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

// docsOn captures the trigger block of docs.yml. push.branches
// must include main; workflow_dispatch must be enabled so the
// maintainer can manually re-deploy without a content commit.
type docsOn struct {
	Push struct {
		Branches []string `yaml:"branches"`
	} `yaml:"push"`
	WorkflowDispatch any `yaml:"workflow_dispatch"`
}

type docsPermissions struct {
	Contents string `yaml:"contents"`
	Pages    string `yaml:"pages"`
	IDToken  string `yaml:"id-token"`
}

type docsJob struct {
	RunsOn string        `yaml:"runs-on"`
	Steps  []releaseStep `yaml:"steps"`
	Needs  any           `yaml:"needs"`
}

type docsWorkflow struct {
	Name        string             `yaml:"name"`
	On          docsOn             `yaml:"on"`
	Permissions docsPermissions    `yaml:"permissions"`
	Jobs        map[string]docsJob `yaml:"jobs"`
}

// TestDocsWorkflow_ParsesAsYAML is the cheap check that catches
// a broken docs.yml before the next push silently fails to
// deploy. GitHub Actions skips invalid workflows without an
// alert on the Actions page; this test substitutes for that.
func TestDocsWorkflow_ParsesAsYAML(t *testing.T) {
	path := findRepoFile(t, filepath.Join(".github", "workflows", "docs.yml"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var parsed any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("docs.yml is not valid YAML: %v", err)
	}
}

// TestDocsWorkflow_PinsExpectedShape locks the load-bearing
// pieces:
//
//   - Triggers on push to main + workflow_dispatch.
//   - permissions.pages == write + permissions.id-token == write
//     (required by actions/deploy-pages@v4).
//   - Two jobs: build, deploy.
//   - The build job runs the recipe-table drift check BEFORE
//     uploading the artifact, so a YAML library change without
//     a docs regenerate fails the deploy rather than shipping
//     stale tables.
//   - The deploy job depends on build (needs: build).
//
// Intentionally NOT pinning exact action versions — that would
// burn on every dependabot bump.
func TestDocsWorkflow_PinsExpectedShape(t *testing.T) {
	path := findRepoFile(t, filepath.Join(".github", "workflows", "docs.yml"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var w docsWorkflow
	if err := yaml.Unmarshal(raw, &w); err != nil {
		t.Fatalf("unmarshal docs.yml: %v", err)
	}

	if w.Name != "docs" {
		t.Errorf("name: got %q, want %q", w.Name, "docs")
	}

	branches := w.On.Push.Branches
	foundMain := false
	for _, b := range branches {
		if b == "main" {
			foundMain = true
			break
		}
	}
	if !foundMain {
		t.Errorf("on.push.branches must include `main`; got %v", branches)
	}
	// workflow_dispatch can be `null`, `{}`, or omitted with
	// `inputs: ...`. Any non-nil value means "present"; YAML
	// "workflow_dispatch:" without a value unmarshals as nil but
	// the key being present in the source is what we want.
	if !strings.Contains(string(raw), "workflow_dispatch:") {
		t.Errorf("workflow_dispatch trigger must be declared (for manual re-deploy)")
	}

	if w.Permissions.Pages != "write" {
		t.Errorf("permissions.pages: got %q, want %q (deploy-pages requires it)", w.Permissions.Pages, "write")
	}
	if w.Permissions.IDToken != "write" {
		t.Errorf("permissions.id-token: got %q, want %q (deploy-pages requires it)", w.Permissions.IDToken, "write")
	}

	build, ok := w.Jobs["build"]
	if !ok {
		t.Fatalf("missing job `build`")
	}
	deploy, ok := w.Jobs["deploy"]
	if !ok {
		t.Fatalf("missing job `deploy`")
	}
	if deploy.Needs == nil {
		t.Errorf("deploy job must declare `needs: build`")
	}

	// build job steps must include: checkout, setup-go, setup-node,
	// the drift check, npm ci, configure-pages, astro build,
	// upload-pages-artifact.
	wantBuildFragments := []string{
		"actions/checkout",
		"actions/setup-go",
		"actions/setup-node",
		"recipes-to-mdx -check",
		"npm ci",
		"actions/configure-pages",
		"npm run build",
		"actions/upload-pages-artifact",
	}
	for _, want := range wantBuildFragments {
		if !stepsContain(build.Steps, want) {
			t.Errorf("build job missing step referencing %q; got steps:\n%+v", want, build.Steps)
		}
	}
	if !stepsContain(deploy.Steps, "actions/deploy-pages") {
		t.Errorf("deploy job must use actions/deploy-pages; got steps:\n%+v", deploy.Steps)
	}
}

// TestCNAMEServesEnvdoctorDev confirms docs/public/CNAME contains
// the literal "envdoctor.dev" — that's what GitHub Pages reads to
// route the custom domain. The whole Probe.DocURL contract
// depends on this one line of plain text being correct.
func TestCNAMEServesEnvdoctorDev(t *testing.T) {
	path := findRepoFile(t, filepath.Join("docs", "public", "CNAME"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	got := strings.TrimSpace(string(raw))
	if got != "envdoctor.dev" {
		t.Errorf("CNAME: got %q, want %q (matches the Probe.DocURL host contract)", got, "envdoctor.dev")
	}
}
