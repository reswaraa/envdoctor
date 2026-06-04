// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// RenderOptions controls how a Report is rendered to a TTY.
type RenderOptions struct {
	// Color enables ANSI color sequences.
	Color bool
}

// DefaultRenderOptions reads NO_COLOR, FORCE_COLOR, CI and detects whether
// out is a terminal, producing the standard CLI behavior.
func DefaultRenderOptions(out *os.File) RenderOptions {
	return RenderOptions{Color: WantColor(out)}
}

// WantColor returns whether ANSI color should be emitted given the
// process environment and whether out is a TTY.
//
// Rules (highest precedence first):
//
//   - NO_COLOR set        → never color (https://no-color.org/)
//   - FORCE_COLOR set     → always color
//   - CI set              → never color (CI logs are read mostly via files)
//   - stdout is a TTY     → color
//   - otherwise           → no color
func WantColor(out *os.File) bool {
	return wantColorImpl(
		os.Getenv("NO_COLOR"),
		os.Getenv("FORCE_COLOR"),
		os.Getenv("CI"),
		isTTY(out),
	)
}

func wantColorImpl(noColor, forceColor, ci string, tty bool) bool {
	if noColor != "" {
		return false
	}
	if forceColor != "" {
		return true
	}
	if ci != "" {
		return false
	}
	return tty
}

func isTTY(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

type pen struct{ color bool }

func (p pen) wrap(seq, s string) string {
	if !p.color {
		return s
	}
	return seq + s + ansiReset
}

func (p pen) bold(s string) string   { return p.wrap(ansiBold, s) }
func (p pen) dim(s string) string    { return p.wrap(ansiDim, s) }
func (p pen) red(s string) string    { return p.wrap(ansiRed, s) }
func (p pen) green(s string) string  { return p.wrap(ansiGreen, s) }
func (p pen) yellow(s string) string { return p.wrap(ansiYellow, s) }
func (p pen) cyan(s string) string   { return p.wrap(ansiCyan, s) }

// Render writes a human-readable view of r to w. Pure function over the
// Report (and options); no environment lookups, no time.Now().
func Render(w io.Writer, r *Report, opts RenderOptions) error {
	p := pen{color: opts.Color}

	header := fmt.Sprintf("Scanning %s  %s",
		r.RepoRoot,
		p.dim(fmt.Sprintf("(envdoctor %s)", r.EnvdoctorVersion)),
	)
	if _, err := fmt.Fprintln(w, p.bold(header)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, p.dim(systemLine(r.System))); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if len(r.Findings) == 0 {
		if _, err := fmt.Fprintln(w, p.green("✓ No problems found.")); err != nil {
			return err
		}
	} else {
		byCat := groupByCategory(r.Findings)
		cats := make([]string, 0, len(byCat))
		for c := range byCat {
			cats = append(cats, c)
		}
		sort.Strings(cats)
		for _, c := range cats {
			if err := renderCategory(w, p, c, byCat[c]); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	summary := summaryLine(r.Findings, r.FinishedAt.Sub(r.StartedAt))
	_, err := fmt.Fprintln(w, p.dim(summary))
	return err
}

func systemLine(s System) string {
	parts := []string{s.OS + "/" + s.Arch}
	if s.Distro != "" {
		parts = append(parts, s.Distro)
	}
	if s.WSL {
		parts = append(parts, "wsl")
	}
	if s.Shell != "" {
		parts = append(parts, s.Shell)
	}
	return strings.Join(parts, ", ")
}

func groupByCategory(fs []Finding) map[string][]Finding {
	g := map[string][]Finding{}
	for _, f := range fs {
		g[f.Category] = append(g[f.Category], f)
	}
	return g
}

func renderCategory(w io.Writer, p pen, cat string, fs []Finding) error {
	if _, err := fmt.Fprintln(w, p.bold(cat)); err != nil {
		return err
	}
	for _, f := range fs {
		if err := renderFinding(w, p, f); err != nil {
			return err
		}
	}
	return nil
}

func renderFinding(w io.Writer, p pen, f Finding) error {
	icon, colored := statusIcon(p, f)
	line := fmt.Sprintf("  %s %s", colored(icon), f.Summary)
	if _, err := fmt.Fprintln(w, line); err != nil {
		return err
	}

	fields := []struct{ k, v string }{
		{"observed", f.Observed},
		{"expected", f.Expected},
		{"evidence", strings.Join(f.Evidence, ", ")},
		{"fix", f.RecipeCommand},
		{"docs", f.DocURL},
	}
	for _, kv := range fields {
		if kv.v == "" {
			continue
		}
		label := p.dim(fmt.Sprintf("    %-9s", kv.k+":"))
		if _, err := fmt.Fprintf(w, "%s %s\n", label, kv.v); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func statusIcon(p pen, f Finding) (string, func(string) string) {
	switch f.Status {
	case StatusOK:
		return "✓", p.green
	case StatusFail:
		switch f.Severity {
		case SeverityWarning:
			return "⚠", p.yellow
		default:
			return "✗", p.red
		}
	case StatusSkipped:
		return "⊘", p.dim
	case StatusProbeFailed:
		return "‼", p.cyan
	}
	return "?", p.dim
}

func summaryLine(fs []Finding, dur interface{ String() string }) string {
	var errs, warns, failed int
	for _, f := range fs {
		switch f.Status {
		case StatusProbeFailed:
			failed++
		case StatusFail:
			if f.Severity == SeverityWarning {
				warns++
			} else {
				errs++
			}
		}
	}
	parts := []string{}
	if errs > 0 {
		parts = append(parts, fmt.Sprintf("%d error", errs))
		if errs > 1 {
			parts[len(parts)-1] += "s"
		}
	}
	if warns > 0 {
		parts = append(parts, fmt.Sprintf("%d warning", warns))
		if warns > 1 {
			parts[len(parts)-1] += "s"
		}
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d probe failure", failed))
		if failed > 1 {
			parts[len(parts)-1] += "s"
		}
	}
	tail := "all clear"
	if len(parts) > 0 {
		tail = strings.Join(parts, ", ")
	}
	return fmt.Sprintf("Scan finished in %s. %s.", dur.String(), tail)
}
