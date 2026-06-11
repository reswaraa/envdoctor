// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reswaraa/envdoctor/internal/bundle"
	"github.com/reswaraa/envdoctor/internal/output"
)

func TestExplain_RoundTripsSavedBundle(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "b.json")

	// Build a Bundle with a known Finding and write it.
	r := &output.Report{
		SchemaVersion:    output.SchemaVersion,
		EnvdoctorVersion: "0.1.0",
		RepoRoot:         "/repo",
		System:           output.System{OS: "darwin", Arch: "arm64"},
		Findings: []output.Finding{
			{
				ID:       "node-version-1",
				Probe:    "node-version",
				Category: output.CategoryRuntime,
				Severity: output.SeverityError,
				Status:   output.StatusFail,
				Summary:  "Node 18.17.0 detected; repo requires 20.10.0",
				DocURL:   "https://reswaraa.github.io/envdoctor/probes/node-version",
			},
		},
	}
	b := bundle.New("0.1.0", r, "abc123def456")
	if _, err := bundle.WritePath(p, b, bundle.RedactOptions{IncludePaths: true}); err != nil {
		t.Fatalf("WritePath: %v", err)
	}

	// Run explain.
	cmd := newExplainCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{p})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("explain: %v", err)
	}

	// Pretty renderer pins these specific strings.
	for _, want := range []string{
		"Scanning /repo",
		"Node 18.17.0",
		"https://reswaraa.github.io/envdoctor/probes/node-version",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing %q; got:\n%s", want, stdout.String())
		}
	}
	if !strings.Contains(stderr.String(), "abc123def456"[:12]) {
		t.Errorf("stderr should surface short recipe_hash; got %q", stderr.String())
	}
}

func TestExplain_JSONFlagEmitsReportJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "b.json")
	b := bundle.New("0.1.0", &output.Report{
		SchemaVersion:    output.SchemaVersion,
		EnvdoctorVersion: "0.1.0",
		RepoRoot:         "/r",
		System:           output.System{OS: "linux", Arch: "amd64"},
		Findings:         []output.Finding{},
	}, "")
	if _, err := bundle.WritePath(p, b, bundle.RedactOptions{IncludePaths: true}); err != nil {
		t.Fatal(err)
	}

	cmd := newExplainCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--json", p})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("explain --json: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout.String()), "{") {
		t.Errorf("--json must emit JSON on stdout; got:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Scanning") {
		t.Errorf("--json must NOT emit pretty TTY; got:\n%s", stdout.String())
	}
}

func TestExplain_MissingFileIsCrash(t *testing.T) {
	cmd := newExplainCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"/no/such/path.json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing path")
	}
	var ec *exitErr
	if !errors.As(err, &ec) {
		t.Fatalf("expected exitErr; got %T", err)
	}
	if ec.code != ExitCrashed {
		t.Errorf("exit code: got %d, want ExitCrashed", ec.code)
	}
}

func TestExplain_BundleWithoutReportIsCrash(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "b.json")
	// Construct a malformed-but-parseable bundle (no embedded report).
	bad := []byte(`{"schema_version":"1","envdoctor_version":"x","generated_at":"2026-06-09T00:00:00Z"}`)
	if err := writeFile(p, bad); err != nil {
		t.Fatal(err)
	}

	cmd := newExplainCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{p})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for report-less bundle")
	}
}

func writeFile(path string, content []byte) error {
	return os.WriteFile(path, content, 0o644)
}
