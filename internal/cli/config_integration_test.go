// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestScan_DisableFilterDropsFindings constructs a tempdir with a
// Node manifest (which would normally surface a node-version finding
// when the host Node doesn't match) plus an .envdoctor.yaml that
// disables the node-version probe. The resulting Report must NOT
// contain any node-version findings.
func TestScan_DisableFilterDropsFindings(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, ".nvmrc", "99.99.99\n") // host will not have v99
	mustWrite(t, dir, ".envdoctor.yaml", `schema_version: 1
disable:
  - node-version
`)
	report, err := runScan(context.Background(), dir, scanFlags{})
	if err != nil {
		t.Fatalf("runScan: %v", err)
	}
	for _, f := range report.Findings {
		if f.Probe == "node-version" {
			t.Errorf("node-version finding must be filtered out; got %+v", f)
		}
	}
}

// TestScan_CustomChecksProduceFindings constructs a tempdir with an
// .envdoctor.yaml requiring a command that definitely isn't on PATH,
// then asserts a custom-probe finding lists the missing command.
func TestScan_CustomChecksProduceFindings(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, ".envdoctor.yaml", `schema_version: 1
checks:
  - type: command_present
    command: definitely-missing-binary-zzz
    reason: "fixture test"
`)
	report, err := runScan(context.Background(), dir, scanFlags{})
	if err != nil {
		t.Fatalf("runScan: %v", err)
	}
	var customFound bool
	for _, f := range report.Findings {
		if f.Probe != "custom" {
			continue
		}
		customFound = true
		if !strings.Contains(f.Summary, "definitely-missing-binary-zzz") {
			t.Errorf("custom Finding should mention the missing command; got %q", f.Summary)
		}
		joined := strings.Join(f.Evidence, "; ")
		if !strings.Contains(joined, "fixture test") {
			t.Errorf("evidence should surface the reason; got %q", joined)
		}
	}
	if !customFound {
		t.Errorf("expected a custom Finding in %+v", report.Findings)
	}
}

// TestScan_MalformedConfigReturnsExitConfigParseError pins the
// behavior: when .envdoctor.yaml is broken, runScan returns an
// exitErr carrying ExitConfigParseError so the caller maps it to
// exit code 4 — distinct from "machine is broken" (1/2).
func TestScan_MalformedConfigReturnsExitConfigParseError(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, ".envdoctor.yaml", "schema_version: 1\nchecks:\n  - type: nope\n")
	_, err := runScan(context.Background(), dir, scanFlags{})
	code, ok := asExitCode(err)
	if !ok {
		t.Fatalf("expected exitErr; got %v", err)
	}
	if code != ExitConfigParseError {
		t.Errorf("code: got %d, want %d", code, ExitConfigParseError)
	}
}

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
