// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Command recipes-to-mdx auto-generates the per-probe recipe
// table on each probe doc page. It walks every probe page under
// docs/src/content/docs/probes/, finds the
//
//	<!-- BEGIN auto-recipes -->
//	...anything...
//	<!-- END auto-recipes -->
//
// block, and replaces the contents with a Markdown table sourced
// from internal/recipes/library/*.yaml.
//
// Usage:
//
//	go run ./scripts/recipes-to-mdx            # rewrite in place
//	go run ./scripts/recipes-to-mdx -check     # CI drift check
//
// The -check mode exits non-zero if any file would change. The
// docs CI workflow runs `-check` before the build so a YAML library
// change without a docs regenerate fails CI loudly.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/reswaraa/envdoctor/internal/recipes"
)

const (
	beginMarker = "<!-- BEGIN auto-recipes -->"
	endMarker   = "<!-- END auto-recipes -->"
)

// markerRe matches the whole block including the markers
// themselves so a single ReplaceAllString swaps the section.
var markerRe = regexp.MustCompile(`(?s)<!-- BEGIN auto-recipes -->.*?<!-- END auto-recipes -->`)

func main() {
	var docsDir string
	var check bool
	flag.StringVar(&docsDir, "docs", "docs/src/content/docs/probes", "probes content directory")
	flag.BoolVar(&check, "check", false, "exit non-zero if any file would change (CI drift gate)")
	flag.Parse()

	lib, err := recipes.DefaultLibrary()
	if err != nil {
		log.Fatalf("load recipes: %v", err)
	}

	byProbe := map[string][]recipes.Recipe{}
	for _, r := range lib.Recipes {
		byProbe[r.Probe] = append(byProbe[r.Probe], r)
	}

	files, err := filepath.Glob(filepath.Join(docsDir, "*.md"))
	if err != nil {
		log.Fatalf("glob probes dir: %v", err)
	}
	sort.Strings(files)

	driftCount := 0
	for _, f := range files {
		probeID := strings.TrimSuffix(filepath.Base(f), ".md")
		body := renderProbeRecipes(byProbe[probeID])
		changed, ok, applyErr := applyMarkers(f, body, check)
		if applyErr != nil {
			log.Fatalf("%s: %v", f, applyErr)
		}
		if !ok {
			log.Fatalf("%s: missing %s / %s markers", f, beginMarker, endMarker)
		}
		if changed {
			driftCount++
			rel, _ := filepath.Rel(".", f)
			fmt.Fprintf(os.Stderr, "  %s %s\n", changeWord(check), rel)
		}
	}
	if check && driftCount > 0 {
		fmt.Fprintf(os.Stderr, "recipes-to-mdx: %d page(s) out of sync with internal/recipes/library — run `go run ./scripts/recipes-to-mdx`\n", driftCount)
		os.Exit(1)
	}
}

func changeWord(checkMode bool) string {
	if checkMode {
		return "would update"
	}
	return "updated"
}

// renderProbeRecipes is the table-emission core. Pulled into a
// pure function (no I/O) so the test can pin its output without
// touching the filesystem.
func renderProbeRecipes(rs []recipes.Recipe) string {
	if len(rs) == 0 {
		return "_No recipes today — open an issue with a debug bundle so a recipe can be authored._\n"
	}
	// Stable order across runs — recipes by ID, fixes in declared
	// order (declared order is significant: the matcher walks
	// non-fallback fixes first, then fallback fixes, in declaration
	// order; preserving it in the table mirrors what the matcher
	// will pick).
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID })
	var sb strings.Builder
	for i, r := range rs {
		if i > 0 {
			sb.WriteString("\n")
		}
		if len(rs) > 1 {
			fmt.Fprintf(&sb, "### `%s`\n\n", r.ID)
		}
		sb.WriteString("| Fix | Class | When | Fallback |\n")
		sb.WriteString("|---|---|---|---|\n")
		for _, f := range r.Fixes {
			fmt.Fprintf(&sb, "| `%s` | %s | %s | %s |\n",
				f.ID, f.Class, summarizeMatch(f.When), boolMark(f.Fallback))
		}
	}
	return sb.String()
}

func summarizeMatch(m recipes.Match) string {
	parts := []string{}
	if m.OS != "" {
		parts = append(parts, "os="+m.OS)
	}
	if m.Arch != "" {
		parts = append(parts, "arch="+m.Arch)
	}
	if m.Distro != "" {
		parts = append(parts, "distro="+m.Distro)
	}
	if m.HasTool != "" {
		parts = append(parts, "has_tool="+m.HasTool)
	}
	if len(parts) == 0 {
		return "*"
	}
	return strings.Join(parts, ", ")
}

func boolMark(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

// applyMarkers reads path, replaces the auto-recipes block with
// body, and writes the result back (unless dryRun). Returns:
//   - changed: true if the file's bytes differ from disk.
//   - ok:      false if the markers aren't present in the file.
//   - err:     non-nil for I/O errors.
//
// The `ok=false` branch is the safety net for a probe page that
// hasn't been edited to include the markers yet — we refuse to
// silently leave it ungenerated.
func applyMarkers(path, body string, dryRun bool) (changed, ok bool, err error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, false, err
	}
	if !bytes.Contains(src, []byte(beginMarker)) || !bytes.Contains(src, []byte(endMarker)) {
		return false, false, nil
	}
	newSection := beginMarker + "\n\n" + body + "\n" + endMarker
	updated := markerRe.ReplaceAllString(string(src), newSection)
	if updated == string(src) {
		return false, true, nil
	}
	if dryRun {
		return true, true, nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return false, true, err
	}
	return true, true, nil
}
