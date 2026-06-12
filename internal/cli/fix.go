// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/reswaraa/envdoctor/internal/audit"
	"github.com/reswaraa/envdoctor/internal/config"
	"github.com/reswaraa/envdoctor/internal/engine"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/probes"
	"github.com/reswaraa/envdoctor/internal/recipes"
	"github.com/reswaraa/envdoctor/internal/system"
)

type fixOpts struct {
	yes     bool
	include []string
	dryRun  bool
}

func newFixCmd() *cobra.Command {
	var f fixOpts
	cmd := &cobra.Command{
		Use:   "fix",
		Short: "Repair findings with consent prompts and an audit log",
		Long: `Fix scans the repo, then walks each finding in repair order
(runtime versions → port collisions → docker state → other),
prompting for consent before running each fix command.

Safety classes:
  safe         the recipe only touches the contributor's user-space
               state (e.g. mise install). Prompt default: Yes.
  shared       the recipe touches shared system state (brew). Prompt
               default: No. --yes alone won't auto-run; pass
               --include=shared to widen.
  destructive  the recipe will kill processes or delete state. Prompt
               default: No. --yes won't auto-run; pass
               --include=destructive to widen.
  privileged   the recipe requires sudo. envdoctor never executes
               these — the command is printed for you to run.

Each executed fix is appended to ~/.local/state/envdoctor/audit.log
as a JSON line for forensics.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFix(cmd, f)
		},
	}
	cmd.Flags().BoolVar(&f.yes, "yes", false, "auto-confirm `safe` fixes; widen with --include")
	cmd.Flags().StringSliceVar(&f.include, "include", nil, "classes to auto-confirm alongside `safe` when --yes is set (e.g. --include=shared,destructive)")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "print fix commands without executing them")
	return cmd
}

// runFix is the shared entry point for `envdoctor fix` and the
// future `envdoctor scan --fix` alias. It scans the repo, walks
// findings in repair order, prompts/executes per the consent matrix,
// re-probes after each successful run, and prints a final summary.
func runFix(cmd *cobra.Command, opts fixOpts) error {
	ctx := cmd.Context()
	stdout := cmd.OutOrStdout()
	stderr := cmd.ErrOrStderr()

	cwd, err := os.Getwd()
	if err != nil {
		return &exitErr{code: ExitCrashed, err: fmt.Errorf("resolve cwd: %w", err)}
	}
	repoRoot, err := filepath.Abs(cwd)
	if err != nil {
		return &exitErr{code: ExitCrashed, err: fmt.Errorf("abs cwd: %w", err)}
	}

	facts := system.Collect()
	lib, err := recipes.DefaultLibrary()
	if err != nil {
		return &exitErr{code: ExitCrashed, err: fmt.Errorf("load recipes: %w", err)}
	}
	cfg, err := config.Load(repoRoot, Version)
	if err != nil {
		return &exitErr{code: ExitConfigParseError, err: err}
	}

	report := output.NewReport(Version, repoRoot, facts.AsSystem())
	findings, _ := engine.New(BuiltinProbes(lib, cfg)).Run(ctx, probes.Input{RepoRoot: repoRoot, System: facts})
	report.Findings = filterDisabled(findings, cfg)
	report.Finalize()

	if len(report.Findings) == 0 {
		writef(stdout, "%s — no findings, nothing to fix.\n", repoRoot)
		return nil
	}

	prompter := newTTYPrompter(os.Stdin, stdout)
	runner := newBashRunner(stdout, stderr)
	summary := walkFixes(ctx, walkInput{
		stdout:       stdout,
		stderr:       stderr,
		findings:     sortRepair(report.Findings),
		opts:         opts,
		prompter:     prompter,
		runner:       runner,
		recipeHash:   lib.Hash(),
		appendAudit:  audit.Append,
		reprobeCheck: makeReprober(lib, cfg, repoRoot, facts),
	})

	writef(stdout, "\nFixed: %d. Failed: %d. Skipped: %d. Still broken: %d.\n",
		summary.Fixed, summary.Failed, summary.Skipped, summary.StillBroken)

	if summary.StillBroken > 0 || summary.Failed > 0 {
		return &exitErr{code: ExitRepairable}
	}
	return nil
}

// walkInput bundles the moving parts of the fix loop so its
// signature stays readable and tests can swap out the Prompter /
// Runner / audit / reprobe hooks without dragging cobra or global
// state into the test setup.
type walkInput struct {
	stdout, stderr io.Writer
	findings       []output.Finding
	opts           fixOpts
	prompter       Prompter
	runner         Runner
	recipeHash     string

	// appendAudit is invoked once per executed fix. In production this
	// is audit.Append (writes to ~/.local/state/...); tests pass a
	// no-op so they don't touch the user's real audit log.
	appendAudit func(audit.Entry) error

	// reprobeCheck is called after each successful fix to determine
	// whether the original finding cleared. In production this re-runs
	// the single matching probe; tests pass a constant to avoid running
	// real probes.
	reprobeCheck func(ctx context.Context, probeID, originalSummary string) bool
}

// FixSummary is the final tally surfaced both as the user-visible
// summary line and as the exit-code input.
type FixSummary struct {
	Fixed       int
	Failed      int
	Skipped     int
	StillBroken int
}

func walkFixes(ctx context.Context, w walkInput) FixSummary {
	var s FixSummary
	total := len(w.findings)
	for i, f := range w.findings {
		printFindingHeader(w.stdout, i+1, total, f)

		if f.RecipeCommand == "" {
			// Engine emitted a finding with no recipe — this is the
			// exit-code-2 signal at scan time. At fix time we just
			// count it as still broken and move on; there's nothing
			// to execute.
			writeln(w.stdout, "  no recipe available — envdoctor needs a new one for this case.")
			s.StillBroken++
			continue
		}

		decision := decideClass(f.RecipeClass, w.opts)
		switch decision {
		case DecisionPrintOnly:
			writef(w.stdout, "  print only (class=%s): %s\n", f.RecipeClass, f.RecipeCommand)
			s.Skipped++
			s.StillBroken++
			continue
		case DecisionSkip:
			writef(w.stdout, "  skipped (class=%s not in --include)\n", f.RecipeClass)
			s.Skipped++
			s.StillBroken++
			continue
		case DecisionPrompt:
			ans, err := w.prompter.Confirm("Apply this fix?", defaultAnswer(f.RecipeClass))
			if err != nil {
				writef(w.stderr, "  prompt failed: %v\n", err)
				s.Skipped++
				s.StillBroken++
				continue
			}
			switch ans {
			case AnswerNo, AnswerSkip:
				writeln(w.stdout, "  skipped.")
				s.Skipped++
				s.StillBroken++
				continue
			case AnswerQuit:
				writeln(w.stdout, "  quit. Remaining findings left untouched.")
				// Account for the current finding plus everything not
				// yet visited as still broken.
				s.StillBroken += total - i
				return s
			case AnswerYes:
				// fall through to execution below
			}
		case DecisionRun:
			// Auto-confirm path; fall through to execution.
		}

		writef(w.stdout, "  $ %s\n", f.RecipeCommand)
		result, err := w.runner.Run(ctx, f.RecipeCommand)
		if w.appendAudit != nil {
			auditErr := w.appendAudit(audit.Entry{
				Command:       f.RecipeCommand,
				ExitCode:      result.ExitCode,
				StdoutTail:    result.StdoutTail,
				StderrTail:    result.StderrTail,
				RecipeID:      f.RecipeID,
				RecipeClass:   f.RecipeClass,
				RecipeVersion: shortHash(w.recipeHash),
			})
			if auditErr != nil {
				// Best-effort: a failed audit write doesn't fail the fix.
				writef(w.stderr, "  audit log write failed: %v\n", auditErr)
			}
		}
		if err != nil || result.ExitCode != 0 {
			writef(w.stdout, "  ✗ fix failed (exit %d). Stopping.\n", result.ExitCode)
			s.Failed++
			s.StillBroken++
			// Account for everything not yet visited as still broken.
			s.StillBroken += total - i - 1
			return s
		}

		// Re-probe just this finding's probe and check whether the
		// state cleared.
		cleared := false
		if w.reprobeCheck != nil {
			cleared = w.reprobeCheck(ctx, f.Probe, f.Summary)
		}
		if cleared {
			writeln(w.stdout, "  ✓ fixed.")
			s.Fixed++
		} else {
			writeln(w.stdout, "  ✗ command ran but the finding persists.")
			s.Failed++
			s.StillBroken++
		}
	}
	return s
}

// printFindingHeader writes the per-finding banner shown before
// the prompt. Format mirrors the scan TTY render's per-finding
// block so users see a consistent layout.
func printFindingHeader(w io.Writer, idx, total int, f output.Finding) {
	class := f.RecipeClass
	if class == "" {
		class = "no-recipe"
	}
	writef(w, "\n[%d/%d] %s / %s  [class: %s]\n", idx, total, f.Category, f.Probe, class)
	writef(w, "  ✗ %s\n", f.Summary)
	if f.Observed != "" {
		writef(w, "    observed: %s\n", f.Observed)
	}
	if f.Expected != "" {
		writef(w, "    expected: %s\n", f.Expected)
	}
	if f.DocURL != "" {
		writef(w, "    docs:     %s\n", f.DocURL)
	}
}

// writef and writeln are tiny wrappers that drop the (n, err)
// returns from fmt.Fprintf / fmt.Fprintln. Every write in this
// file goes to the user's TTY; if the terminal disappears mid-fix,
// the whole loop is moot, so we don't propagate the error.
// errcheck-clean by construction.
func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func writeln(w io.Writer, s string) {
	_, _ = fmt.Fprintln(w, s)
}

// shortHash trims the recipe library SHA-256 to 12 hex chars for
// the audit log's recipe_version field — same shortening the
// explain footer uses, so log entries can be cross-referenced
// against bundle reports.
func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// repairOrder maps probe IDs to a priority bucket used to sort
// findings before walking them. Priority order: runtime versions ->
// port collisions -> docker state -> everything else.
// Unknown probe IDs land in bucket 3.
var repairOrder = map[string]int{
	"node-version":   0,
	"python-version": 0,
	"go-version":     0,
	"ruby-version":   0,
	"port-free":      1,
	"docker-running": 2,
}

func repairPriority(probeID string) int {
	if p, ok := repairOrder[probeID]; ok {
		return p
	}
	return 3
}

// sortRepair returns findings sorted into repair order. Stable
// sort: findings with the same priority keep the engine's original
// (alphabetical-by-probe-ID) order.
func sortRepair(findings []output.Finding) []output.Finding {
	out := make([]output.Finding, len(findings))
	copy(out, findings)
	sort.SliceStable(out, func(i, j int) bool {
		return repairPriority(out[i].Probe) < repairPriority(out[j].Probe)
	})
	return out
}

// makeReprober returns a closure that, given a probe ID and the
// original finding's Summary, re-runs that single probe and
// reports whether the Summary still appears among the new
// findings. The factory shape lets walkInput hold a small
// function value rather than carrying lib/cfg/facts/repoRoot
// through the reprobeCheck function.
//
// Re-running one probe (not the whole engine) keeps the post-fix
// check sub-millisecond. Matching by Summary works for every
// current probe because each Finding's Summary is keyed off the
// specific source-of-truth (the missing port, the outdated
// version, the absent command).
func makeReprober(lib *recipes.Library, cfg *config.Config, repoRoot string, facts *system.Facts) func(context.Context, string, string) bool {
	return func(ctx context.Context, probeID, originalSummary string) bool {
		all := BuiltinProbes(lib, cfg)
		var only []probes.Probe
		for _, p := range all {
			if p.ID() == probeID {
				only = []probes.Probe{p}
				break
			}
		}
		if len(only) == 0 {
			return false
		}
		findings, _ := engine.New(only).Run(ctx, probes.Input{RepoRoot: repoRoot, System: facts})
		for _, f := range findings {
			if f.Summary == originalSummary {
				return false
			}
		}
		return true
	}
}
