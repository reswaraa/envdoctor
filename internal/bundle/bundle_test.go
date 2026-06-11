// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/reswaraa/envdoctor/internal/output"
)

func fixedReport() *output.Report {
	start := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	return &output.Report{
		SchemaVersion:    "1",
		EnvdoctorVersion: "0.1.0",
		RepoRoot:         "/Users/alice/work/cool-repo",
		StartedAt:        start,
		FinishedAt:       start.Add(50 * time.Millisecond),
		System:           output.System{OS: "darwin", Arch: "arm64", Shell: "/bin/zsh"},
		Findings: []output.Finding{
			{
				ID:       "node-version-1",
				Probe:    "node-version",
				Category: output.CategoryRuntime,
				Severity: output.SeverityError,
				Status:   output.StatusFail,
				Summary:  "Node 18.17.0 detected (in /Users/alice/work/cool-repo)",
				Observed: "18.17.0",
				Expected: "20.10.0",
				Evidence: []string{".nvmrc"},
				DocURL:   "https://reswaraa.github.io/envdoctor/probes/node-version",
			},
		},
	}
}

func TestNew_SetsSchemaVersionAndTimestamps(t *testing.T) {
	b := New("0.1.0", fixedReport(), "deadbeef")
	if b.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion: got %q, want %q", b.SchemaVersion, SchemaVersion)
	}
	if b.EnvdoctorVersion != "0.1.0" {
		t.Errorf("EnvdoctorVersion: got %q", b.EnvdoctorVersion)
	}
	if b.RecipeHash != "deadbeef" {
		t.Errorf("RecipeHash: got %q", b.RecipeHash)
	}
	if time.Since(b.GeneratedAt) > 5*time.Second {
		t.Errorf("GeneratedAt should be ~now; got %v", b.GeneratedAt)
	}
}

func TestRedact_DefaultStripsRepoRootToBasename(t *testing.T) {
	b := New("0.1.0", fixedReport(), "")
	Redact(b, RedactOptions{})
	if b.Report.RepoRoot != "cool-repo" {
		t.Errorf("RepoRoot: got %q, want %q", b.Report.RepoRoot, "cool-repo")
	}
}

func TestNew_DeepCopiesReport(t *testing.T) {
	r := fixedReport()
	b := New("0.1.0", r, "")
	Redact(b, RedactOptions{})
	// Caller's original Report must not have been mutated by Redact.
	if r.RepoRoot != "/Users/alice/work/cool-repo" {
		t.Errorf("caller's RepoRoot leaked from Redact; got %q", r.RepoRoot)
	}
	if r.Findings[0].Summary != "Node 18.17.0 detected (in /Users/alice/work/cool-repo)" {
		t.Errorf("caller's Finding.Summary leaked from Redact; got %q", r.Findings[0].Summary)
	}
}

func TestRedact_IncludePathsKeepsRepoRoot(t *testing.T) {
	b := New("0.1.0", fixedReport(), "")
	Redact(b, RedactOptions{IncludePaths: true})
	if !strings.HasPrefix(b.Report.RepoRoot, "/") {
		t.Errorf("with IncludePaths, RepoRoot should stay absolute; got %q", b.Report.RepoRoot)
	}
}

func TestRedact_StripsUsersHomePrefixFromFindingStrings(t *testing.T) {
	b := New("0.1.0", fixedReport(), "")
	// Set HOME to the exact prefix the Finding's Summary contains so
	// the test is deterministic regardless of the actual host user.
	t.Setenv("HOME", "/Users/alice")
	Redact(b, RedactOptions{})
	got := b.Report.Findings[0].Summary
	if strings.Contains(got, "/Users/alice") {
		t.Errorf("/Users/alice should be stripped from Summary; got %q", got)
	}
	if !strings.Contains(got, "~/work/cool-repo") {
		t.Errorf("Summary should contain redacted ~/work/...; got %q", got)
	}
}

func TestRedact_PreservesRelativeEvidence(t *testing.T) {
	b := New("0.1.0", fixedReport(), "")
	Redact(b, RedactOptions{})
	if b.Report.Findings[0].Evidence[0] != ".nvmrc" {
		t.Errorf("project-relative evidence must survive redaction; got %q",
			b.Report.Findings[0].Evidence[0])
	}
}

func TestRedact_NilSafe(_ *testing.T) {
	Redact(nil, RedactOptions{}) // must not panic
	Redact(&Bundle{}, RedactOptions{})
}

func TestWritePath_RoundTrip(t *testing.T) {
	t.Setenv("HOME", "/Users/alice")

	dir := t.TempDir()
	p := filepath.Join(dir, "envdoctor-bundle.json")
	b := New("0.1.0", fixedReport(), "abc123")

	stats, err := WritePath(p, b, RedactOptions{})
	if err != nil {
		t.Fatalf("WritePath: %v", err)
	}
	if stats.SizeBytes == 0 {
		t.Errorf("SizeBytes: got 0")
	}
	if stats.Findings != 1 {
		t.Errorf("Findings: got %d, want 1", stats.Findings)
	}
	if stats.EnvValues != 0 || stats.FileBodies != 0 {
		t.Errorf("structural guarantee: EnvValues=%d FileBodies=%d, both must be 0", stats.EnvValues, stats.FileBodies)
	}

	// Round-trip via Read.
	got, err := Read(p)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.RecipeHash != "abc123" {
		t.Errorf("RecipeHash: got %q, want abc123", got.RecipeHash)
	}
	if got.Report.RepoRoot != "cool-repo" {
		t.Errorf("on-disk RepoRoot should be redacted; got %q", got.Report.RepoRoot)
	}
}

func TestWritePath_ContainsNoAbsoluteUserPaths(t *testing.T) {
	t.Setenv("HOME", "/Users/alice")

	dir := t.TempDir()
	p := filepath.Join(dir, "b.json")
	b := New("0.1.0", fixedReport(), "")
	if _, err := WritePath(p, b, RedactOptions{}); err != nil {
		t.Fatalf("WritePath: %v", err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "/Users/alice") {
		t.Errorf("bundle on disk must not contain /Users/alice; first 400 bytes:\n%s",
			string(raw[:min(400, len(raw))]))
	}
}

func TestStats_PreviewLineFormat(t *testing.T) {
	s := Stats{SizeBytes: 1234, Findings: 2, Tools: 0, EnvValues: 0, FileBodies: 0}
	line := s.PreviewLine("/tmp/b.json")
	// Pin the contract: the line MUST surface env value count and
	// file body count so a contributor reading it understands what
	// they're sharing.
	for _, want := range []string{"2 finding", "0 env value", "0 file content", "/tmp/b.json"} {
		if !strings.Contains(line, want) {
			t.Errorf("preview line missing %q; got %q", want, line)
		}
	}
}

func TestBundle_RoundTripsViaEncodingJSON(t *testing.T) {
	in := New("0.1.0", fixedReport(), "abc")
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Bundle
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.SchemaVersion != in.SchemaVersion {
		t.Errorf("round-trip SchemaVersion: got %q", out.SchemaVersion)
	}
	if out.Report.SchemaVersion != "1" {
		t.Errorf("nested Report SchemaVersion: got %q", out.Report.SchemaVersion)
	}
}
