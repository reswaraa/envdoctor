// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"

	"github.com/reswaraa/envdoctor/internal/recipes"
)

// Answer is one of the four single-key responses the fix prompt
// accepts. Skip is functionally the same as No today but is exposed
// to the user so the prompt UX has a clear "skip this and move on"
// verb separate from "no, I'm second-guessing whether to fix at all."
type Answer int

// Answer values for the y/n/s/q prompt. The single-character tokens
// the user types are case-insensitive ("Y" works, so does "yes").
const (
	AnswerYes Answer = iota
	AnswerNo
	AnswerSkip
	AnswerQuit
)

// Prompter is the interface for `envdoctor fix`'s consent UX. The
// production implementation reads a line from stdin and writes the
// prompt to stdout (ttyPrompter); tests supply a scripted version.
type Prompter interface {
	Confirm(prompt string, defaultYes bool) (Answer, error)
}

// ttyPrompter reads one line from in and parses the first non-blank
// token as a y/n/s/q answer. An empty line takes the indicated
// default; anything unrecognized is treated as No (the cautious
// default for destructive-ish prompts).
type ttyPrompter struct {
	in     *bufio.Reader
	out    io.Writer
	closed bool
}

func newTTYPrompter(in io.Reader, out io.Writer) *ttyPrompter {
	return &ttyPrompter{in: bufio.NewReader(in), out: out}
}

func (p *ttyPrompter) Confirm(prompt string, defaultYes bool) (Answer, error) {
	if p.closed {
		return AnswerNo, errors.New("prompter closed")
	}
	indicator := "[y/N/s/q]"
	if defaultYes {
		indicator = "[Y/n/s/q]"
	}
	if _, err := fmt.Fprintf(p.out, "  %s %s: ", prompt, indicator); err != nil {
		return AnswerNo, err
	}
	line, err := p.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return AnswerNo, err
	}
	if errors.Is(err, io.EOF) {
		// User closed stdin (Ctrl-D); treat as Quit so the loop exits
		// cleanly rather than hanging or misinterpreting as No.
		p.closed = true
		return AnswerQuit, nil
	}
	return parseAnswer(line, defaultYes), nil
}

func parseAnswer(line string, defaultYes bool) Answer {
	s := strings.TrimSpace(strings.ToLower(line))
	if s == "" {
		if defaultYes {
			return AnswerYes
		}
		return AnswerNo
	}
	switch s {
	case "y", "yes":
		return AnswerYes
	case "n", "no":
		return AnswerNo
	case "s", "skip":
		return AnswerSkip
	case "q", "quit", "exit":
		return AnswerQuit
	default:
		return AnswerNo
	}
}

// RunResult is what Runner.Run returns. StdoutTail / StderrTail are
// captures of the subprocess output for the audit log; the live
// streams are tee'd to the user's TTY at run time.
type RunResult struct {
	ExitCode   int
	StdoutTail string
	StderrTail string
}

// Runner is the interface for executing a fix command. The
// production implementation invokes `bash -c <command>`; tests
// supply a scripted version that records calls and returns fixed
// exit codes without running real shell commands.
type Runner interface {
	Run(ctx context.Context, command string) (RunResult, error)
}

// bashRunner shells out to /bin/bash -c. Subprocess stdout/stderr
// are tee'd to the user's TTY (stdout, stderr) so a long install
// is visible in real time, while a captured copy of the trailing
// bytes is returned for the audit log.
type bashRunner struct {
	stdout, stderr io.Writer
}

func newBashRunner(stdout, stderr io.Writer) *bashRunner {
	return &bashRunner{stdout: stdout, stderr: stderr}
}

func (r *bashRunner) Run(ctx context.Context, command string) (RunResult, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(r.stdout, &outBuf)
	cmd.Stderr = io.MultiWriter(r.stderr, &errBuf)

	runErr := cmd.Run()
	res := RunResult{StdoutTail: outBuf.String(), StderrTail: errBuf.String()}

	if runErr == nil {
		return res, nil
	}
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		res.ExitCode = ee.ExitCode()
		// Non-zero exit is not a Go-level error — the fix ran, it just
		// didn't succeed. Callers distinguish via ExitCode.
		return res, nil
	}
	// The shell itself couldn't start (bash missing, ctx cancelled,
	// filesystem error). Surface as -1 so the caller knows the
	// command didn't even execute.
	res.ExitCode = -1
	return res, fmt.Errorf("run fix: %w", runErr)
}

// FixDecision is the four-way outcome of applying the consent
// matrix to a single Finding. It's pure of UI and side effects so
// the table-driven test in fix_test.go can exercise the full
// flag/class combinatorial space.
type FixDecision int

// FixDecision values driven by Finding.RecipeClass and the fix
// flags (--yes, --include, --dry-run).
const (
	// DecisionRun: execute the command without prompting.
	DecisionRun FixDecision = iota
	// DecisionPrompt: ask the user y/n/s/q.
	DecisionPrompt
	// DecisionPrintOnly: print the command, do not execute (privileged
	// class and --dry-run). Anti-feature: envdoctor never auto-runs
	// sudo, even when the user types `y`.
	DecisionPrintOnly
	// DecisionSkip: skip silently. Used when --yes was set but the
	// finding's class isn't covered by --include and the user opted
	// into non-interactive mode.
	DecisionSkip
)

// decideClass maps (class, flags) onto a FixDecision. The matrix:
//
//	class\flag        no flags    --yes      --yes --include=<class>   --dry-run
//	safe              prompt(Y)   run        run                       print
//	shared            prompt(N)   skip       run                       print
//	destructive       prompt(N)   skip       run                       print
//	privileged        print       print      print                     print
//
// "prompt(Y)" means a prompt whose default-on-enter is Yes; the
// caller passes defaultYes to the Prompter.
func decideClass(class string, opts fixOpts) FixDecision {
	if opts.dryRun {
		return DecisionPrintOnly
	}
	if class == string(recipes.ClassPrivileged) {
		// Anti-feature: privileged is print-only at every flag combo.
		// The user must execute it themselves.
		return DecisionPrintOnly
	}
	if !opts.yes {
		return DecisionPrompt
	}
	if class == string(recipes.ClassSafe) {
		return DecisionRun
	}
	if slices.Contains(opts.include, class) {
		return DecisionRun
	}
	// --yes was given but this class isn't in --include — skip
	// silently rather than prompting (the user explicitly opted into
	// non-interactive mode).
	return DecisionSkip
}

// defaultAnswer is the prompt default keyed off recipe class:
// safe defaults Yes (the common one-tap path), everything else
// defaults No (forces an explicit y for shared/destructive).
func defaultAnswer(class string) bool {
	return class == string(recipes.ClassSafe)
}
