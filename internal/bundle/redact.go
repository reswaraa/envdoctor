// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package bundle

import (
	"os"
	"os/user"
	"strings"
)

// RedactOptions controls how aggressively a Bundle is stripped of
// host-identifying paths before write. Defaults are aggressive —
// the opt-outs exist for the rare maintainer who needs raw paths
// to diagnose a bug.
type RedactOptions struct {
	// IncludePaths keeps absolute filesystem paths verbatim instead
	// of replacing $HOME with "~" and stripping RepoRoot to its
	// basename. Surfaced as `--bundle-include-paths` on the scan
	// command.
	IncludePaths bool
}

// Redact mutates b in place, removing host-identifying information.
// The transforms applied (in order):
//
//  1. RepoRoot              → basename only (unless IncludePaths)
//  2. $HOME prefix          → "~"
//  3. /Users/<user> prefix  → "~"  (macOS shape, even when $HOME differs)
//  4. /home/<user> prefix   → "~"  (Linux shape)
//  5. Current username      → "$USER" wherever it survived above
//  6. System.Kernel         → unchanged (no user info)
//  7. Report.System.Shell   → unchanged (path is generic)
//
// Findings.Evidence entries that start with a project-relative path
// (e.g. ".nvmrc", "package.json#engines.node") are left alone — by
// probe convention they're already repo-relative.
//
// EnvValues / file bodies are NOT touched here: the probe layer is
// the structural guarantee that those never enter a Finding in the
// first place (Phase 2 env_required design). This function is the
// belt-and-suspenders for paths, not values.
func Redact(b *Bundle, opts RedactOptions) {
	if b == nil || b.Report == nil {
		return
	}
	prefixes := redactPrefixes()
	username := currentUser()

	if !opts.IncludePaths {
		b.Report.RepoRoot = basename(b.Report.RepoRoot)
	}

	for i := range b.Report.Findings {
		f := &b.Report.Findings[i]
		f.Summary = scrub(f.Summary, prefixes, username)
		f.Observed = scrub(f.Observed, prefixes, username)
		f.Expected = scrub(f.Expected, prefixes, username)
		f.RecipeCommand = scrub(f.RecipeCommand, prefixes, username)
		for j, ev := range f.Evidence {
			f.Evidence[j] = scrub(ev, prefixes, username)
		}
	}
}

// scrub applies the path-collapse transforms to a single string.
// Order matters: $HOME first (catches most), then /Users/... and
// /home/... (catches paths from other accounts on shared CI hosts),
// then the username substitution as a final pass.
func scrub(s string, prefixes []string, username string) string {
	if s == "" {
		return s
	}
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		s = strings.ReplaceAll(s, p, "~")
	}
	if username != "" && username != "root" {
		s = strings.ReplaceAll(s, "/"+username, "/$USER")
		s = strings.ReplaceAll(s, username, "$USER")
	}
	return s
}

func redactPrefixes() []string {
	prefixes := []string{}
	if home := os.Getenv("HOME"); home != "" {
		prefixes = append(prefixes, home)
	}
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		prefixes = append(prefixes, u.HomeDir)
		// Also strip canonical platform prefixes even when $HOME
		// has been remapped (containers, runners): /Users/<u> and
		// /home/<u> are the two real-world shapes.
		if u.Username != "" {
			prefixes = append(prefixes, "/Users/"+u.Username)
			prefixes = append(prefixes, "/home/"+u.Username)
		}
	}
	return prefixes
}

func currentUser() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return ""
}

func basename(p string) string {
	if p == "" {
		return p
	}
	// Trim trailing slashes.
	for len(p) > 1 && p[len(p)-1] == '/' {
		p = p[:len(p)-1]
	}
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
