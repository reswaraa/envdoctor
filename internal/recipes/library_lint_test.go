// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package recipes

import (
	"strings"
	"testing"
)

// TestLibrary_NoFixWrapsSudo enforces that envdoctor never wraps sudo. A non-privileged Fix whose Command
// begins with `sudo ` is misclassified — either move the Fix to
// class=privileged (which envdoctor will then print but never
// execute), or rewrite it to not require sudo at all.
//
// This is the structural backstop for the consent matrix in
// fix_runner.go: even if a future contributor adds a Fix that
// auto-runs (safe/shared/destructive) and contains sudo, this
// test fails first so it never ships.
func TestLibrary_NoFixWrapsSudo(t *testing.T) {
	lib, err := DefaultLibrary()
	if err != nil {
		t.Fatalf("DefaultLibrary: %v", err)
	}
	for _, r := range lib.Recipes {
		for _, f := range r.Fixes {
			if f.Class == ClassPrivileged {
				// Privileged commands are printed verbatim for the
				// user to run; they're expected to contain sudo.
				continue
			}
			cmd := strings.TrimSpace(f.Command)
			if strings.HasPrefix(cmd, "sudo ") {
				t.Errorf("recipe %q fix %q (class=%s) starts with `sudo ` — must be class=privileged\n  command: %s",
					r.ID, f.ID, f.Class, cmd)
			}
		}
	}
}

// TestLibrary_PrivilegedFixesExist makes sure the print-only path
// is actually exercised by the shipped library: if this count
// drops to zero, either we genuinely removed every sudo recipe
// (bump the test) or someone misclassified them and the no-sudo
// guard above has nothing to protect.
func TestLibrary_PrivilegedFixesExist(t *testing.T) {
	lib, err := DefaultLibrary()
	if err != nil {
		t.Fatalf("DefaultLibrary: %v", err)
	}
	count := 0
	for _, r := range lib.Recipes {
		for _, f := range r.Fixes {
			if f.Class == ClassPrivileged {
				count++
			}
		}
	}
	if count == 0 {
		t.Errorf("expected at least one Fix with class=privileged; got 0 — has the library lost its sudo entries?")
	}
}

// TestLibrary_EveryFixClassIsValid is belt-and-suspenders on top
// of validateRecipe (which already rejects unknown classes at
// load time). If validation regresses, this test catches it at
// the snapshot level.
func TestLibrary_EveryFixClassIsValid(t *testing.T) {
	lib, err := DefaultLibrary()
	if err != nil {
		t.Fatalf("DefaultLibrary: %v", err)
	}
	valid := map[Class]bool{
		ClassSafe:        true,
		ClassShared:      true,
		ClassDestructive: true,
		ClassPrivileged:  true,
	}
	for _, r := range lib.Recipes {
		for _, f := range r.Fixes {
			if !valid[f.Class] {
				t.Errorf("recipe %q fix %q has invalid class %q", r.ID, f.ID, f.Class)
			}
		}
	}
}

// TestLibrary_SudoOnlyAppearsInPrivilegedFixes is the other side
// of NoFixWrapsSudo: any Fix Command that mentions `sudo`
// *anywhere* must be classified privileged. A `sudo` deep in the
// middle of a shell pipeline counts; we don't want envdoctor to
// auto-run anything that escalates partway through.
func TestLibrary_SudoOnlyAppearsInPrivilegedFixes(t *testing.T) {
	lib, err := DefaultLibrary()
	if err != nil {
		t.Fatalf("DefaultLibrary: %v", err)
	}
	for _, r := range lib.Recipes {
		for _, f := range r.Fixes {
			if f.Class == ClassPrivileged {
				continue
			}
			if mentionsSudo(f.Command) {
				t.Errorf("recipe %q fix %q (class=%s) command references sudo — must be class=privileged\n  command: %s",
					r.ID, f.ID, f.Class, f.Command)
			}
		}
	}
}

// mentionsSudo reports whether `sudo` appears as its own word in
// s. We don't want to false-positive on tokens like "pseudoterm"
// or "kasudo", so we look for the substring with word-edge
// boundaries (whitespace or shell separators).
func mentionsSudo(s string) bool {
	const tok = "sudo"
	for i := 0; i+len(tok) <= len(s); i++ {
		if s[i:i+len(tok)] != tok {
			continue
		}
		if i > 0 && !isShellBoundary(s[i-1]) {
			continue
		}
		if i+len(tok) < len(s) && !isShellBoundary(s[i+len(tok)]) {
			continue
		}
		return true
	}
	return false
}

func isShellBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', ';', '|', '&', '(', ')', '`':
		return true
	}
	return false
}

func TestMentionsSudo_WordBoundaries(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"sudo apt-get install", true},
		{"echo hi && sudo systemctl start docker", true},
		{"sudo", true},
		{"(sudo apt install)", true},
		{"`sudo apt`", true},
		{"echo pseudoterm", false}, // substring, not a word
		{"echo kasudo", false},     // suffix substring
		{"echo not-sudo-prefix", false},
		{"echo studo", false}, // false positive insurance
		{"", false},
	}
	for _, c := range cases {
		if got := mentionsSudo(c.s); got != c.want {
			t.Errorf("mentionsSudo(%q): got %v, want %v", c.s, got, c.want)
		}
	}
}
