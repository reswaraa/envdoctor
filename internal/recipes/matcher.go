// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package recipes

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/reswaraa/envdoctor/internal/system"
)

// SelectFix picks the best Fix from r for the given system facts and
// renders the Fix's Command template against params. Returns:
//
//   - (chosenFix, renderedCommand, nil) on success.
//   - (zero Fix, "", nil) when no fix's Match clause is satisfied.
//   - (zero Fix, "", err) when the chosen Fix's template fails to render
//     (a bug in the recipe library, not in the user's environment).
//
// Selection rules (first-match-wins, two-pass):
//
//  1. Iterate fixes in declaration order. The first non-fallback fix
//     whose When clause matches the facts wins.
//  2. If no non-fallback fix matched, iterate again and return the
//     first fallback fix whose When clause matches.
//
// This lets a recipe encode "prefer mise/fnm/nvm/asdf, fall back to
// brew" by ordering the preferred fixes first and marking brew with
// fallback: true.
func SelectFix(r Recipe, facts *system.Facts, params map[string]any) (Fix, string, error) {
	chosen := pickFix(r, facts)
	if chosen == nil {
		return Fix{}, "", nil
	}
	cmd, err := expandTemplate(chosen.Command, params)
	if err != nil {
		return Fix{}, "", fmt.Errorf("recipe %q fix %q: %w", r.ID, chosen.ID, err)
	}
	return *chosen, cmd, nil
}

func pickFix(r Recipe, facts *system.Facts) *Fix {
	for i := range r.Fixes {
		f := &r.Fixes[i]
		if f.Fallback {
			continue
		}
		if matches(f.When, facts) {
			return f
		}
	}
	for i := range r.Fixes {
		f := &r.Fixes[i]
		if !f.Fallback {
			continue
		}
		if matches(f.When, facts) {
			return f
		}
	}
	return nil
}

// matches reports whether m is satisfied by facts. Empty fields in m
// are wildcards. HasTool is checked via facts.HasTool, which uses the
// shared mutex-guarded cache.
func matches(m Match, facts *system.Facts) bool {
	if m.OS != "" && m.OS != facts.OS {
		return false
	}
	if m.Arch != "" && m.Arch != facts.Arch {
		return false
	}
	if m.Distro != "" && m.Distro != facts.Distro {
		return false
	}
	if m.HasTool != "" && !facts.HasTool(m.HasTool) {
		return false
	}
	return true
}

// expandTemplate renders tmpl against params using text/template with
// missingkey=error. A typo in the recipe key name fails loudly rather
// than producing a silently empty command.
func expandTemplate(tmpl string, params map[string]any) (string, error) {
	t, err := template.New("cmd").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}
