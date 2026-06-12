// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReport_RoundTrip(t *testing.T) {
	in := &Report{
		SchemaVersion:    SchemaVersion,
		EnvdoctorVersion: "0.1.0-test",
		RepoRoot:         "/path/to/repo",
		StartedAt:        time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC),
		FinishedAt:       time.Date(2026, 6, 4, 12, 0, 1, 0, time.UTC),
		System: System{
			OS:     "darwin",
			Arch:   "arm64",
			Shell:  "/bin/zsh",
			Kernel: "Darwin 25.2.0",
		},
		Findings: []Finding{
			{
				ID:            "node-version-1",
				Probe:         "node-version",
				Category:      CategoryRuntime,
				Severity:      SeverityError,
				Status:        StatusFail,
				Summary:       "Node 18.17.0 detected; repo requires ^20.0.0",
				Observed:      "18.17.0",
				Expected:      "^20.0.0",
				Evidence:      []string{".nvmrc"},
				RecipeID:      "mise-install-node",
				RecipeClass:   "safe",
				RecipeCommand: "mise install node@20.10.0",
				DocURL:        "https://reswaraa.github.io/envdoctor/probes/node-version-mismatch",
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, in); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var out Report
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if out.SchemaVersion != in.SchemaVersion {
		t.Errorf("SchemaVersion: got %q, want %q", out.SchemaVersion, in.SchemaVersion)
	}
	if !out.StartedAt.Equal(in.StartedAt) {
		t.Errorf("StartedAt: got %v, want %v", out.StartedAt, in.StartedAt)
	}
	if !out.FinishedAt.Equal(in.FinishedAt) {
		t.Errorf("FinishedAt: got %v, want %v", out.FinishedAt, in.FinishedAt)
	}
	if out.System != in.System {
		t.Errorf("System mismatch: got %+v, want %+v", out.System, in.System)
	}
	if len(out.Findings) != 1 {
		t.Fatalf("Findings len: got %d, want 1", len(out.Findings))
	}
	if out.Findings[0].Probe != in.Findings[0].Probe {
		t.Errorf("Finding.Probe: got %q, want %q", out.Findings[0].Probe, in.Findings[0].Probe)
	}
	if out.Findings[0].DocURL != in.Findings[0].DocURL {
		t.Errorf("Finding.DocURL: got %q, want %q", out.Findings[0].DocURL, in.Findings[0].DocURL)
	}
	// RecipeClass is part of the canonical on-wire schema; renaming
	// any of safe/shared/destructive/privileged is an incompatible change.
	if out.Findings[0].RecipeClass != "safe" {
		t.Errorf("Finding.RecipeClass round-trip: got %q, want %q", out.Findings[0].RecipeClass, "safe")
	}
	raw := buf.String()
	if !strings.Contains(raw, `"recipe_class": "safe"`) {
		t.Errorf("on-wire JSON must use snake_case `recipe_class`; got:\n%s", raw)
	}
}

func TestReport_EmptyFindingsSerializeAsArray(t *testing.T) {
	r := NewReport("0.0.0", ".", System{OS: "linux", Arch: "amd64"})
	r.Finalize()

	var buf bytes.Buffer
	if err := WriteJSON(&buf, r); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `"findings": []`) {
		t.Errorf("empty findings must serialize as []; got:\n%s", got)
	}
	if strings.Contains(got, `"findings": null`) {
		t.Errorf("findings must never serialize as null; got:\n%s", got)
	}
}

func TestReport_GoldenJSON(t *testing.T) {
	fixed := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)
	r := &Report{
		SchemaVersion:    "1",
		EnvdoctorVersion: "0.1.0",
		RepoRoot:         "/repo",
		StartedAt:        fixed,
		FinishedAt:       fixed.Add(time.Second),
		System: System{
			OS:    "darwin",
			Arch:  "arm64",
			Shell: "/bin/zsh",
		},
		Findings: []Finding{},
	}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, r); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	want := `{
  "schema_version": "1",
  "envdoctor_version": "0.1.0",
  "repo_root": "/repo",
  "started_at": "2026-06-04T00:00:00Z",
  "finished_at": "2026-06-04T00:00:01Z",
  "system": {
    "os": "darwin",
    "arch": "arm64",
    "shell": "/bin/zsh",
    "wsl": false
  },
  "findings": []
}
`
	if buf.String() != want {
		t.Errorf("golden JSON mismatch.\n--- got ---\n%s\n--- want ---\n%s", buf.String(), want)
	}
}

// TestSchemaConstants pins the string values of the constants that ship in
// the JSON output. Renaming any of these is an incompatible schema change.
// If this test fails, you almost certainly need to bump SchemaVersion and
// add a migration path, not "fix" the test.
func TestSchemaConstants(t *testing.T) {
	cases := []struct {
		got, want string
	}{
		{string(SeverityError), "error"},
		{string(SeverityWarning), "warning"},
		{string(SeverityInfo), "info"},
		{string(StatusOK), "ok"},
		{string(StatusFail), "fail"},
		{string(StatusSkipped), "skipped"},
		{string(StatusProbeFailed), "probe_failed"},
		{CategoryRuntime, "runtime"},
		{CategoryEnvironment, "environment"},
		{CategoryDocker, "docker"},
		{CategoryPorts, "ports"},
		{CategoryPath, "path"},
		{CategoryArchitecture, "architecture"},
		{CategoryCustom, "custom"},
		{SchemaVersion, "1"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("schema constant drift: got %q, want %q", c.got, c.want)
		}
	}
}
