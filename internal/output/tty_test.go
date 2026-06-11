// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWantColor_Precedence(t *testing.T) {
	cases := []struct {
		name       string
		noColor    string
		forceColor string
		ci         string
		tty        bool
		want       bool
	}{
		{"noColor wins over force", "1", "1", "", true, false},
		{"noColor wins over tty", "1", "", "", true, false},
		{"force beats ci", "", "1", "true", false, true},
		{"force beats no-tty", "", "1", "", false, true},
		{"ci beats tty", "", "", "true", true, false},
		{"tty alone enables", "", "", "", true, true},
		{"no env, no tty", "", "", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := wantColorImpl(c.noColor, c.forceColor, c.ci, c.tty); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestRender_EmptyReport_NoColor(t *testing.T) {
	r := fixedReport()
	var buf bytes.Buffer
	if err := Render(&buf, r, RenderOptions{Color: false}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "Scanning /repo  (envdoctor 0.1.0)\n" +
		"darwin/arm64, /bin/zsh\n" +
		"\n" +
		"✓ No problems found.\n" +
		"\n" +
		"Scan finished in 250ms. all clear.\n"
	if buf.String() != want {
		t.Errorf("render mismatch.\n--- got ---\n%s\n--- want ---\n%s", buf.String(), want)
	}
}

func TestRender_OneFailing_NoColor(t *testing.T) {
	r := fixedReport()
	r.Findings = []Finding{
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
			RecipeCommand: "mise install node@20.10.0",
			DocURL:        "https://reswaraa.github.io/envdoctor/probes/node-version-mismatch",
		},
	}
	var buf bytes.Buffer
	if err := Render(&buf, r, RenderOptions{Color: false}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "Scanning /repo  (envdoctor 0.1.0)\n" +
		"darwin/arm64, /bin/zsh\n" +
		"\n" +
		"runtime\n" +
		"  ✗ Node 18.17.0 detected; repo requires ^20.0.0\n" +
		"    observed: 18.17.0\n" +
		"    expected: ^20.0.0\n" +
		"    evidence: .nvmrc\n" +
		"    fix:      mise install node@20.10.0\n" +
		"    docs:     https://reswaraa.github.io/envdoctor/probes/node-version-mismatch\n" +
		"\n" +
		"\n" +
		"Scan finished in 250ms. 1 error.\n"
	if buf.String() != want {
		t.Errorf("render mismatch.\n--- got ---\n%s\n--- want ---\n%s", buf.String(), want)
	}
}

func TestRender_GroupsByCategory(t *testing.T) {
	r := fixedReport()
	r.Findings = []Finding{
		{Probe: "ports", Category: CategoryPorts, Severity: SeverityWarning, Status: StatusFail, Summary: "Port 5432 in use", DocURL: "x"},
		{Probe: "node-version", Category: CategoryRuntime, Severity: SeverityError, Status: StatusFail, Summary: "Node too old", DocURL: "x"},
		{Probe: "docker", Category: CategoryDocker, Severity: SeverityError, Status: StatusFail, Summary: "Docker not running", DocURL: "x"},
	}
	var buf bytes.Buffer
	if err := Render(&buf, r, RenderOptions{Color: false}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	dockerAt := strings.Index(out, "docker\n")
	portsAt := strings.Index(out, "ports\n")
	runtimeAt := strings.Index(out, "runtime\n")
	if dockerAt < 0 || portsAt < 0 || runtimeAt < 0 {
		t.Fatalf("missing category headers:\n%s", out)
	}
	if dockerAt >= portsAt || portsAt >= runtimeAt {
		t.Errorf("categories not alphabetical: docker=%d ports=%d runtime=%d", dockerAt, portsAt, runtimeAt)
	}
}

func TestRender_ColorEmitsANSI(t *testing.T) {
	r := fixedReport()
	r.Findings = []Finding{{
		Probe:    "x",
		Category: "runtime",
		Severity: SeverityError,
		Status:   StatusFail,
		Summary:  "broken",
		DocURL:   "x",
	}}
	var buf bytes.Buffer
	if err := Render(&buf, r, RenderOptions{Color: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), ansiRed) {
		t.Errorf("expected red ANSI for an error finding; got:\n%s", buf.String())
	}
}

func TestRender_NoColorEmitsNoANSI(t *testing.T) {
	r := fixedReport()
	r.Findings = []Finding{{
		Probe: "x", Category: "runtime", Severity: SeverityError, Status: StatusFail,
		Summary: "broken", DocURL: "x",
	}}
	var buf bytes.Buffer
	if err := Render(&buf, r, RenderOptions{Color: false}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("Color=false must emit no ANSI sequences; got:\n%q", buf.String())
	}
}

func TestRender_SummaryPluralization(t *testing.T) {
	r := fixedReport()
	r.Findings = []Finding{
		{Probe: "a", Category: "x", Severity: SeverityError, Status: StatusFail, Summary: "1", DocURL: "x"},
		{Probe: "b", Category: "x", Severity: SeverityError, Status: StatusFail, Summary: "2", DocURL: "x"},
		{Probe: "c", Category: "x", Severity: SeverityWarning, Status: StatusFail, Summary: "3", DocURL: "x"},
	}
	var buf bytes.Buffer
	if err := Render(&buf, r, RenderOptions{Color: false}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "2 errors") {
		t.Errorf("expected '2 errors'; got:\n%s", got)
	}
	if !strings.Contains(got, "1 warning") {
		t.Errorf("expected '1 warning' (singular); got:\n%s", got)
	}
}

// --- helpers ---

func fixedReport() *Report {
	start := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	return &Report{
		SchemaVersion:    "1",
		EnvdoctorVersion: "0.1.0",
		RepoRoot:         "/repo",
		StartedAt:        start,
		FinishedAt:       start.Add(250 * time.Millisecond),
		System: System{
			OS:    "darwin",
			Arch:  "arm64",
			Shell: "/bin/zsh",
		},
		Findings: []Finding{},
	}
}
