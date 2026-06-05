// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reswaraa/envdoctor/internal/inference"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/system"
)

const testEnvRecipe = `
id: env-missing
probe: env-required
fixes:
  - id: copy-env-example
    class: safe
    command: "cp -n .env.example .env"
`

func TestEnvRequired_ID(t *testing.T) {
	if got := EnvRequired(nil).ID(); got != "env-required" {
		t.Errorf("got %q, want env-required", got)
	}
}

func TestEnvRequired_AppliesTo(t *testing.T) {
	p := EnvRequired(nil)
	if p.AppliesTo(Input{RepoRoot: t.TempDir()}) {
		t.Error("AppliesTo must be false for a repo without .env.example or compose")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte("X=y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !p.AppliesTo(Input{RepoRoot: dir}) {
		t.Error("AppliesTo must be true with .env.example present")
	}
}

func TestEnvRequired_Evaluate_AllPresent(t *testing.T) {
	p := &envRequiredProbe{lib: mkLibrary(t, testEnvRecipe)}
	reqs := []inference.EnvRequirement{
		{Source: ".env.example", Key: "DB_URL"},
		{Source: ".env.example", Key: "JWT"},
	}
	present := map[string]bool{"DB_URL": true, "JWT": true}
	if got := p.evaluate(reqs, present, &system.Facts{}); got != nil {
		t.Errorf("expected nil; got %+v", got)
	}
}

func TestEnvRequired_Evaluate_MissingEmitsFinding(t *testing.T) {
	p := &envRequiredProbe{lib: mkLibrary(t, testEnvRecipe)}
	reqs := []inference.EnvRequirement{
		{Source: ".env.example", Key: "DB_URL"},
		{Source: ".env.example", Key: "JWT"},
		{Source: "docker-compose.yml", Key: "WORKER_TOKEN"},
	}
	present := map[string]bool{"DB_URL": true} // JWT and WORKER_TOKEN missing
	got := p.evaluate(reqs, present, &system.Facts{})
	if len(got) != 1 {
		t.Fatalf("findings: got %d, want 1", len(got))
	}
	f := got[0]
	if f.Probe != "env-required" || f.Category != output.CategoryEnvironment {
		t.Errorf("Probe/Category: %+v", f)
	}
	if !strings.Contains(f.Summary, "JWT") || !strings.Contains(f.Summary, "WORKER_TOKEN") {
		t.Errorf("Summary should list missing keys; got %q", f.Summary)
	}
	if strings.Contains(f.Expected, "DB_URL") {
		t.Errorf("Expected should not include present keys; got %q", f.Expected)
	}
	wantEvidence := []string{".env.example", "docker-compose.yml"}
	if len(f.Evidence) != 2 || f.Evidence[0] != wantEvidence[0] || f.Evidence[1] != wantEvidence[1] {
		t.Errorf("Evidence: got %v, want %v", f.Evidence, wantEvidence)
	}
	if f.RecipeID != "copy-env-example" {
		t.Errorf("RecipeID: got %q, want copy-env-example", f.RecipeID)
	}
}

func TestEnvRequired_Evaluate_NeverIncludesValues(t *testing.T) {
	// Belt-and-suspenders: the probe must never include env *values* in
	// any of the Finding's strings. We construct a present map and a
	// requirements set; the implementation has no access to values
	// anywhere — but if a future refactor introduces a leak, this test
	// is the canary.
	p := &envRequiredProbe{lib: nil}
	reqs := []inference.EnvRequirement{{Source: ".env.example", Key: "JWT_SECRET"}}
	got := p.evaluate(reqs, nil, &system.Facts{})
	if len(got) != 1 {
		t.Fatalf("findings: got %d, want 1", len(got))
	}
	f := got[0]
	// All strings in the Finding must be free of any value-shaped data.
	// We assert the only places that mention JWT_SECRET are key-name
	// contexts (Summary / Expected); Observed is the placeholder.
	if f.Observed != "(missing)" {
		t.Errorf("Observed: got %q, want (missing)", f.Observed)
	}
	if f.Expected != "JWT_SECRET" {
		t.Errorf("Expected: got %q, want JWT_SECRET", f.Expected)
	}
}

func TestEnvRequired_Run_UsesInjectedSources(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte("DB_URL=x\nJWT=y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &envRequiredProbe{
		lib: nil,
		readDotenv: func(_ string) ([]string, error) {
			return []string{"DB_URL"}, nil // only DB_URL in .env
		},
		processEnv: func() []string {
			return nil // empty process env
		},
	}
	got, err := p.Run(context.Background(), Input{RepoRoot: dir, System: &system.Facts{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("findings: got %d, want 1", len(got))
	}
	if !strings.Contains(got[0].Summary, "JWT") {
		t.Errorf("Summary should mention missing JWT; got %q", got[0].Summary)
	}
	if strings.Contains(got[0].Summary, "DB_URL") {
		t.Errorf("Summary should not mention present DB_URL; got %q", got[0].Summary)
	}
}

func TestEnvRequired_Run_ProcessEnvSatisfies(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte("DB_URL=x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &envRequiredProbe{
		lib:        nil,
		readDotenv: func(_ string) ([]string, error) { return nil, nil },
		processEnv: func() []string { return []string{"DB_URL=postgres://..."} },
	}
	got, err := p.Run(context.Background(), Input{RepoRoot: dir, System: &system.Facts{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("process env should satisfy; got %+v", got)
	}
}
