// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/reswaraa/envdoctor/internal/audit"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/recipes"
)

// recordingAuditor captures audit.Entry calls so tests can pin
// fields without touching the user's real audit.log file.
type recordingAuditor struct {
	entries []audit.Entry
	err     error
}

func (a *recordingAuditor) Append(e audit.Entry) error {
	a.entries = append(a.entries, e)
	return a.err
}

// constReprober returns a fixed result every call. Tests use this
// in lieu of running real probes — the consent / loop logic is
// what's under test, not the probe machinery.
type constReprober struct {
	clears bool
	calls  int
}

func (r *constReprober) Check(_ context.Context, _, _ string) bool {
	r.calls++
	return r.clears
}

// --- decideClass: the full consent matrix --------------------------

func TestDecideClass_Matrix(t *testing.T) {
	cases := []struct {
		name  string
		class string
		opts  fixOpts
		want  FixDecision
	}{
		// --dry-run wins over everything.
		{"dryrun beats yes-safe", string(recipes.ClassSafe), fixOpts{yes: true, dryRun: true}, DecisionPrintOnly},
		{"dryrun beats yes-priv", string(recipes.ClassPrivileged), fixOpts{yes: true, dryRun: true}, DecisionPrintOnly},

		// privileged is print-only at every flag combo (anti-feature).
		{"priv no flags", string(recipes.ClassPrivileged), fixOpts{}, DecisionPrintOnly},
		{"priv with yes", string(recipes.ClassPrivileged), fixOpts{yes: true}, DecisionPrintOnly},
		{"priv with yes+include", string(recipes.ClassPrivileged), fixOpts{yes: true, include: []string{"privileged"}}, DecisionPrintOnly},

		// No --yes: every non-privileged class prompts.
		{"safe interactive", string(recipes.ClassSafe), fixOpts{}, DecisionPrompt},
		{"shared interactive", string(recipes.ClassShared), fixOpts{}, DecisionPrompt},
		{"destructive interactive", string(recipes.ClassDestructive), fixOpts{}, DecisionPrompt},

		// --yes: safe runs, others skip unless in --include.
		{"yes runs safe", string(recipes.ClassSafe), fixOpts{yes: true}, DecisionRun},
		{"yes skips shared", string(recipes.ClassShared), fixOpts{yes: true}, DecisionSkip},
		{"yes skips destructive", string(recipes.ClassDestructive), fixOpts{yes: true}, DecisionSkip},

		// --yes + --include widens.
		{"yes+include=shared runs shared", string(recipes.ClassShared), fixOpts{yes: true, include: []string{"shared"}}, DecisionRun},
		{"yes+include=destructive runs destructive", string(recipes.ClassDestructive), fixOpts{yes: true, include: []string{"destructive"}}, DecisionRun},
		{"yes+include=destructive still skips shared", string(recipes.ClassShared), fixOpts{yes: true, include: []string{"destructive"}}, DecisionSkip},
		{"yes+include with multiple", string(recipes.ClassDestructive), fixOpts{yes: true, include: []string{"shared", "destructive"}}, DecisionRun},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decideClass(c.class, c.opts); got != c.want {
				t.Errorf("decideClass(%q, %+v): got %v, want %v", c.class, c.opts, got, c.want)
			}
		})
	}
}

func TestDefaultAnswer_OnlySafeDefaultsYes(t *testing.T) {
	if !defaultAnswer(string(recipes.ClassSafe)) {
		t.Errorf("safe must default to Yes")
	}
	for _, c := range []recipes.Class{recipes.ClassShared, recipes.ClassDestructive, recipes.ClassPrivileged} {
		if defaultAnswer(string(c)) {
			t.Errorf("%q must default to No", c)
		}
	}
}

// --- parseAnswer ---------------------------------------------------

func TestParseAnswer(t *testing.T) {
	cases := []struct {
		in           string
		defaultYes   bool
		want         Answer
		wantContains string
	}{
		{"y\n", false, AnswerYes, ""},
		{"Y\n", false, AnswerYes, ""},
		{"yes\n", false, AnswerYes, ""},
		{"n\n", true, AnswerNo, ""},
		{"NO\n", true, AnswerNo, ""},
		{"s\n", false, AnswerSkip, ""},
		{"skip\n", false, AnswerSkip, ""},
		{"q\n", false, AnswerQuit, ""},
		{"quit\n", false, AnswerQuit, ""},
		{"\n", false, AnswerNo, ""},        // empty + default=N → No
		{"\n", true, AnswerYes, ""},        // empty + default=Y → Yes
		{"  Y  \n", false, AnswerYes, ""},  // whitespace tolerated
		{"unknown\n", false, AnswerNo, ""}, // unknown token → safe default (No)
		{"unknown\n", true, AnswerNo, ""},  // unknown does NOT use defaultYes
	}
	for _, c := range cases {
		got := parseAnswer(c.in, c.defaultYes)
		if got != c.want {
			t.Errorf("parseAnswer(%q, defaultYes=%v): got %v, want %v", c.in, c.defaultYes, got, c.want)
		}
	}
}

// --- sortRepair ----------------------------------------------------

func TestSortRepair_PutsRuntimeFirstThenPortsThenDocker(t *testing.T) {
	findings := []output.Finding{
		{Probe: "env-required"},
		{Probe: "docker-running"},
		{Probe: "port-free"},
		{Probe: "node-version"},
		{Probe: "path-command"},
		{Probe: "python-version"},
	}
	got := sortRepair(findings)
	wantOrder := []string{"node-version", "python-version", "port-free", "docker-running", "env-required", "path-command"}
	for i, w := range wantOrder {
		if got[i].Probe != w {
			t.Errorf("position %d: got %q, want %q", i, got[i].Probe, w)
		}
	}
}

func TestSortRepair_StableWithinBucket(t *testing.T) {
	// Two same-priority findings keep input order.
	findings := []output.Finding{
		{Probe: "python-version", Summary: "first python"},
		{Probe: "node-version", Summary: "first node"},
		{Probe: "ruby-version", Summary: "first ruby"},
	}
	got := sortRepair(findings)
	// All three are priority 0 — order must be preserved.
	if got[0].Summary != "first python" || got[1].Summary != "first node" || got[2].Summary != "first ruby" {
		t.Errorf("stable sort broke input order: %+v", got)
	}
}

// --- walkFixes integration with injected Prompter & Runner ---------

type scriptedPrompter struct {
	answers []Answer
	calls   int
}

func (p *scriptedPrompter) Confirm(_ string, _ bool) (Answer, error) {
	if p.calls >= len(p.answers) {
		return AnswerNo, nil
	}
	a := p.answers[p.calls]
	p.calls++
	return a, nil
}

type recordingRunner struct {
	exitCode   int
	stdoutTail string
	calls      []string
	err        error
}

func (r *recordingRunner) Run(_ context.Context, command string) (RunResult, error) {
	r.calls = append(r.calls, command)
	return RunResult{ExitCode: r.exitCode, StdoutTail: r.stdoutTail}, r.err
}

func newWalkFinding(probe, class, command, summary string) output.Finding {
	return output.Finding{
		ID:            probe + "-1",
		Probe:         probe,
		Category:      "runtime",
		Severity:      output.SeverityError,
		Status:        output.StatusFail,
		Summary:       summary,
		RecipeID:      "test-fix",
		RecipeClass:   class,
		RecipeCommand: command,
		DocURL:        "https://reswaraa.github.io/envdoctor/probes/" + probe,
	}
}

// newTestWalk builds a walkInput with audit + reprobe injected so
// tests never write to the user's real audit.log and never run
// real probes. clearOnReprobe controls what the canned reprober
// reports back.
func newTestWalk(stdout, stderr io.Writer, findings []output.Finding, opts fixOpts, prompter Prompter, runner Runner, auditor *recordingAuditor, reprober *constReprober) walkInput {
	return walkInput{
		stdout:       stdout,
		stderr:       stderr,
		findings:     findings,
		opts:         opts,
		prompter:     prompter,
		runner:       runner,
		recipeHash:   "deadbeefcafe",
		appendAudit:  auditor.Append,
		reprobeCheck: reprober.Check,
	}
}

func TestWalkFixes_DryRunDoesNotExecute(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &recordingRunner{}
	auditor := &recordingAuditor{}
	reprober := &constReprober{}
	w := newTestWalk(&stdout, &stderr,
		[]output.Finding{newWalkFinding("node-version", "safe", "echo hi", "Node old")},
		fixOpts{dryRun: true},
		&scriptedPrompter{}, runner, auditor, reprober,
	)
	got := walkFixes(t.Context(), w)
	if len(runner.calls) != 0 {
		t.Errorf("dry-run must not execute; runner saw %v", runner.calls)
	}
	if len(auditor.entries) != 0 {
		t.Errorf("dry-run must not audit; auditor saw %d entries", len(auditor.entries))
	}
	if got.Skipped != 1 || got.StillBroken != 1 {
		t.Errorf("dry-run summary: got %+v, want Skipped=1 StillBroken=1", got)
	}
	if !strings.Contains(stdout.String(), "print only") {
		t.Errorf("stdout should mention `print only`; got:\n%s", stdout.String())
	}
}

func TestWalkFixes_YesAutoRunsSafeAndAuditsIt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &recordingRunner{exitCode: 0, stdoutTail: "ok"}
	auditor := &recordingAuditor{}
	reprober := &constReprober{clears: true}
	w := newTestWalk(&stdout, &stderr,
		[]output.Finding{
			newWalkFinding("node-version", "safe", "echo safe", "S1"),
			newWalkFinding("port-free", "shared", "echo shared", "S2"),
		},
		fixOpts{yes: true},
		&scriptedPrompter{}, runner, auditor, reprober,
	)
	got := walkFixes(t.Context(), w)

	if len(runner.calls) != 1 || runner.calls[0] != "echo safe" {
		t.Errorf("only safe should auto-run; runner saw %v", runner.calls)
	}
	if got.Fixed != 1 {
		t.Errorf("safe with successful re-probe should count Fixed; got %+v", got)
	}
	if got.Skipped != 1 {
		t.Errorf("shared with --yes (no include) should be Skipped; got %+v", got)
	}
	if len(auditor.entries) != 1 {
		t.Fatalf("audit should record the one execution; got %d entries", len(auditor.entries))
	}
	e := auditor.entries[0]
	if e.Command != "echo safe" {
		t.Errorf("audit Command: got %q", e.Command)
	}
	if e.RecipeClass != "safe" {
		t.Errorf("audit RecipeClass: got %q", e.RecipeClass)
	}
	if e.RecipeVersion != "deadbeefcafe" {
		t.Errorf("audit RecipeVersion (short hash): got %q, want %q", e.RecipeVersion, "deadbeefcafe")
	}
	if e.StdoutTail != "ok" {
		t.Errorf("audit StdoutTail: got %q", e.StdoutTail)
	}
}

func TestWalkFixes_PromptYesExecutesAndPromptNoSkips(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &recordingRunner{exitCode: 0}
	auditor := &recordingAuditor{}
	reprober := &constReprober{clears: true}
	w := newTestWalk(&stdout, &stderr,
		[]output.Finding{
			newWalkFinding("node-version", "shared", "echo yes-please", "S1"),
			newWalkFinding("port-free", "destructive", "echo nope", "S2"),
		},
		fixOpts{}, // no --yes → both prompt
		&scriptedPrompter{answers: []Answer{AnswerYes, AnswerNo}}, runner, auditor, reprober,
	)
	got := walkFixes(t.Context(), w)
	if len(runner.calls) != 1 || runner.calls[0] != "echo yes-please" {
		t.Errorf("only the y-answered fix should run; runner saw %v", runner.calls)
	}
	if got.Skipped != 1 || got.Fixed != 1 {
		t.Errorf("expected Fixed=1 Skipped=1; got %+v", got)
	}
}

func TestWalkFixes_QuitAccountsRemainingAsStillBroken(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &recordingRunner{}
	auditor := &recordingAuditor{}
	reprober := &constReprober{}
	w := newTestWalk(&stdout, &stderr,
		[]output.Finding{
			newWalkFinding("node-version", "shared", "echo a", "S1"),
			newWalkFinding("port-free", "shared", "echo b", "S2"),
			newWalkFinding("docker-running", "shared", "echo c", "S3"),
		},
		fixOpts{},
		&scriptedPrompter{answers: []Answer{AnswerQuit}}, runner, auditor, reprober,
	)
	got := walkFixes(t.Context(), w)
	if len(runner.calls) != 0 {
		t.Errorf("quit on first finding must not execute anything; runner saw %v", runner.calls)
	}
	if got.StillBroken != 3 {
		t.Errorf("all 3 findings must be counted StillBroken on early quit; got %+v", got)
	}
}

func TestWalkFixes_NonZeroExitStopsLoopAndAudits(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &recordingRunner{exitCode: 1, stdoutTail: "boom"}
	auditor := &recordingAuditor{}
	reprober := &constReprober{}
	w := newTestWalk(&stdout, &stderr,
		[]output.Finding{
			newWalkFinding("node-version", "safe", "echo broken", "S1"),
			newWalkFinding("port-free", "safe", "echo never-runs", "S2"),
		},
		fixOpts{yes: true},
		&scriptedPrompter{}, runner, auditor, reprober,
	)
	got := walkFixes(t.Context(), w)
	if len(runner.calls) != 1 {
		t.Errorf("loop must stop after first failure; runner saw %v", runner.calls)
	}
	if got.Failed != 1 {
		t.Errorf("the failing fix should be counted Failed; got %+v", got)
	}
	if got.StillBroken != 2 {
		t.Errorf("both findings should be counted StillBroken (failed + unvisited); got %+v", got)
	}
	// The audit must capture the failure too, not just successes.
	if len(auditor.entries) != 1 || auditor.entries[0].ExitCode != 1 {
		t.Errorf("audit must record the failed fix with exit_code=1; got %+v", auditor.entries)
	}
}

func TestWalkFixes_MissingRecipeIsAccountedAsStillBroken(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &recordingRunner{}
	auditor := &recordingAuditor{}
	reprober := &constReprober{}
	f := newWalkFinding("node-version", "", "", "S1") // no recipe
	w := newTestWalk(&stdout, &stderr,
		[]output.Finding{f},
		fixOpts{},
		&scriptedPrompter{}, runner, auditor, reprober,
	)
	got := walkFixes(t.Context(), w)
	if got.StillBroken != 1 {
		t.Errorf("recipe-less finding must count as StillBroken; got %+v", got)
	}
	if len(runner.calls) != 0 {
		t.Errorf("must not execute anything; runner saw %v", runner.calls)
	}
	if len(auditor.entries) != 0 {
		t.Errorf("no recipe = no execution = no audit; auditor saw %d", len(auditor.entries))
	}
}

// --- ttyPrompter EOF handling --------------------------------------

func TestTTYPrompter_EOFOnStdinIsQuit(t *testing.T) {
	in := strings.NewReader("") // immediate EOF
	out := &bytes.Buffer{}
	p := newTTYPrompter(in, out)
	got, err := p.Confirm("Apply?", false)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if got != AnswerQuit {
		t.Errorf("EOF must map to Quit, not %v", got)
	}
}

func TestTTYPrompter_DefaultIndicatorMatchesDefault(t *testing.T) {
	cases := []struct {
		defaultYes bool
		wantBanner string
	}{
		{true, "[Y/n/s/q]"},
		{false, "[y/N/s/q]"},
	}
	for _, c := range cases {
		out := &bytes.Buffer{}
		p := newTTYPrompter(strings.NewReader("y\n"), out)
		_, _ = p.Confirm("Apply?", c.defaultYes)
		if !strings.Contains(out.String(), c.wantBanner) {
			t.Errorf("defaultYes=%v: expected banner %q in %q", c.defaultYes, c.wantBanner, out.String())
		}
	}
}

// Compile-time check that bashRunner implements Runner.
var _ Runner = (*bashRunner)(nil)
var _ Prompter = (*ttyPrompter)(nil)

// Belt-and-suspenders: ensure errors imported elsewhere keep
// compiling even after refactors.
var _ = errors.New
var _ io.Reader = (*strings.Reader)(nil)
