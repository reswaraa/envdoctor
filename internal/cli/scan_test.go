// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reswaraa/envdoctor/internal/output"
)

func TestRunScan_EmptyReturnsCleanReport(t *testing.T) {
	dir := t.TempDir()
	report, err := runScan(context.Background(), dir, scanFlags{})
	if err != nil {
		t.Fatalf("runScan: %v", err)
	}
	if report.RepoRoot != dir {
		t.Errorf("RepoRoot: got %q, want %q", report.RepoRoot, dir)
	}
	if len(report.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(report.Findings))
	}
	if report.SchemaVersion != output.SchemaVersion {
		t.Errorf("SchemaVersion: got %q, want %q", report.SchemaVersion, output.SchemaVersion)
	}
}

func TestEmitReport_BundleWritesJSON(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "bundle.json")
	r := &output.Report{
		SchemaVersion:    output.SchemaVersion,
		EnvdoctorVersion: "test",
		RepoRoot:         dir,
		System:           output.System{OS: "darwin", Arch: "arm64"},
		Findings:         []output.Finding{},
	}
	var stdout strings.Builder
	if err := emitReport(&stdout, r, scanFlags{bundle: bundlePath, jsonOut: true}); err != nil {
		t.Fatalf("emitReport: %v", err)
	}

	got, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	var parsed output.Report
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("parse bundle: %v", err)
	}
	if parsed.SchemaVersion != output.SchemaVersion {
		t.Errorf("bundle SchemaVersion: got %q, want %q", parsed.SchemaVersion, output.SchemaVersion)
	}
}

func TestEmitReport_JSONFlagWritesOnlyJSONToStdout(t *testing.T) {
	r := &output.Report{
		SchemaVersion:    output.SchemaVersion,
		EnvdoctorVersion: "test",
		RepoRoot:         "/r",
		System:           output.System{OS: "linux", Arch: "amd64"},
		Findings:         []output.Finding{},
	}
	var stdout strings.Builder
	if err := emitReport(&stdout, r, scanFlags{jsonOut: true}); err != nil {
		t.Fatalf("emitReport: %v", err)
	}
	s := stdout.String()
	if !strings.HasPrefix(strings.TrimSpace(s), "{") {
		t.Errorf("--json must emit JSON-only to stdout; got:\n%s", s)
	}
	if strings.Contains(s, "Scanning ") {
		t.Errorf("--json must NOT emit the pretty TTY header; got:\n%s", s)
	}
}

func TestEmitReport_DefaultEmitsPretty(t *testing.T) {
	r := &output.Report{
		SchemaVersion:    output.SchemaVersion,
		EnvdoctorVersion: "test",
		RepoRoot:         "/r",
		System:           output.System{OS: "linux", Arch: "amd64"},
		Findings:         []output.Finding{},
	}
	var stdout strings.Builder
	if err := emitReport(&stdout, r, scanFlags{}); err != nil {
		t.Fatalf("emitReport: %v", err)
	}
	if !strings.Contains(stdout.String(), "Scanning /r") {
		t.Errorf("default mode must emit the pretty header; got:\n%s", stdout.String())
	}
}
