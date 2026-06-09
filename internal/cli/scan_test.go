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

	"github.com/reswaraa/envdoctor/internal/bundle"
	"github.com/reswaraa/envdoctor/internal/output"
)

func TestRunScan_EmptyReturnsCleanReport(t *testing.T) {
	dir := t.TempDir()
	report, recipeHash, err := runScan(context.Background(), dir, scanFlags{})
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
	if recipeHash == "" {
		t.Error("recipeHash must be set so bundles can record which library was active")
	}
}

func TestEmitReport_BundleWritesWrappedBundleJSON(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "bundle.json")
	r := &output.Report{
		SchemaVersion:    output.SchemaVersion,
		EnvdoctorVersion: "test",
		RepoRoot:         dir,
		System:           output.System{OS: "darwin", Arch: "arm64"},
		Findings:         []output.Finding{},
	}
	var stdout, stderr strings.Builder
	err := emitReport(&stdout, &stderr, r, "deadbeef", scanFlags{bundle: bundlePath, jsonOut: true})
	if err != nil {
		t.Fatalf("emitReport: %v", err)
	}

	// The on-disk artifact is a Bundle wrapper, not a bare Report.
	raw, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	var parsed bundle.Bundle
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse bundle: %v", err)
	}
	if parsed.SchemaVersion != bundle.SchemaVersion {
		t.Errorf("bundle SchemaVersion: got %q, want %q", parsed.SchemaVersion, bundle.SchemaVersion)
	}
	if parsed.RecipeHash != "deadbeef" {
		t.Errorf("bundle RecipeHash: got %q, want deadbeef", parsed.RecipeHash)
	}
	if parsed.Report == nil || parsed.Report.SchemaVersion != output.SchemaVersion {
		t.Errorf("nested Report missing or wrong schema: %+v", parsed.Report)
	}

	// The pre-write preview lands on stderr, not stdout.
	if !strings.Contains(stderr.String(), bundlePath) {
		t.Errorf("preview should name the bundle path on stderr; got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "0 env value") {
		t.Errorf("preview should surface the env-value count; got %q", stderr.String())
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
	var stdout, stderr strings.Builder
	if err := emitReport(&stdout, &stderr, r, "", scanFlags{jsonOut: true}); err != nil {
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
	var stdout, stderr strings.Builder
	if err := emitReport(&stdout, &stderr, r, "", scanFlags{}); err != nil {
		t.Fatalf("emitReport: %v", err)
	}
	if !strings.Contains(stdout.String(), "Scanning /r") {
		t.Errorf("default mode must emit the pretty header; got:\n%s", stdout.String())
	}
}

func TestEmitReport_BundleIncludePathsKeepsRepoRoot(t *testing.T) {
	t.Setenv("HOME", "/Users/alice")
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "bundle.json")
	r := &output.Report{
		SchemaVersion:    output.SchemaVersion,
		EnvdoctorVersion: "test",
		RepoRoot:         "/Users/alice/work/cool-repo",
		System:           output.System{OS: "darwin", Arch: "arm64"},
		Findings:         []output.Finding{},
	}
	var stdout, stderr strings.Builder
	if err := emitReport(&stdout, &stderr, r, "", scanFlags{
		bundle:             bundlePath,
		bundleIncludePaths: true,
		jsonOut:            true,
	}); err != nil {
		t.Fatalf("emitReport: %v", err)
	}
	raw, _ := os.ReadFile(bundlePath)
	if !strings.Contains(string(raw), "/Users/alice/work/cool-repo") {
		t.Errorf("--bundle-include-paths must keep RepoRoot verbatim; got:\n%s", string(raw))
	}
}
