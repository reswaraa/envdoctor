// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package audit writes an append-only JSON-lines record of every
// fix command envdoctor executes. The log lives at
//
//	$XDG_STATE_HOME/envdoctor/audit.log  (when XDG_STATE_HOME is set)
//	$HOME/.local/state/envdoctor/audit.log  (otherwise)
//
// per the XDG Base Directory specification. The directory is created
// on first write with 0o700 permissions; the log file itself is 0o600.
// A fix that runs but whose audit write fails is still a successful
// fix from the user's perspective — audit is best-effort, not a gate.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry is one row of the audit log. The on-wire field names are
// stable; renaming any of them breaks downstream log parsers.
//
// StdoutTail and StderrTail are truncated to the last TailBytes
// bytes of the fix's output so a one-off command that printed
// megabytes can't bloat the audit log.
type Entry struct {
	Timestamp     time.Time `json:"ts"`
	Command       string    `json:"command"`
	ExitCode      int       `json:"exit_code"`
	StdoutTail    string    `json:"stdout_tail,omitempty"`
	StderrTail    string    `json:"stderr_tail,omitempty"`
	RecipeID      string    `json:"recipe_id,omitempty"`
	RecipeClass   string    `json:"recipe_class,omitempty"`
	RecipeVersion string    `json:"recipe_version,omitempty"`
}

// TailBytes is the cap applied to StdoutTail / StderrTail. 4 KiB
// is enough for the message you actually want (the last few lines
// of an installer) without making one chatty fix flood the log.
const TailBytes = 4096

// LogPath returns the resolved audit log path. The lookup order is
// $XDG_STATE_HOME → $HOME → user.HomeDir. An empty string is
// returned when no usable home can be resolved (CI containers with
// no $HOME); callers should treat that as "audit disabled".
func LogPath() string {
	dir := stateDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "envdoctor", "audit.log")
}

func stateDir() string {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return xdg
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "state")
	}
	return ""
}

// Append writes one Entry as JSON + newline to the resolved log
// path. The file is opened O_APPEND | O_CREATE | O_WRONLY with 0o600
// so concurrent fix runs from two terminals don't clobber each
// other (POSIX guarantees atomic appends of writes <= PIPE_BUF).
// The parent directory is created with 0o700 if missing.
//
// Returns nil when audit is disabled (no resolvable home) — that is
// not an error; it's "log silently dropped, fix still succeeded."
// Returns a non-nil error only for unexpected filesystem failures
// (full disk, permission denied on an existing path) which the
// caller should surface to stderr but not block the fix on.
func Append(e Entry) error {
	return AppendTo(LogPath(), e)
}

// AppendTo is Append parameterized by path; tests use it to write
// into a tempdir without touching the real $HOME.
func AppendTo(path string, e Entry) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("audit mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("audit open: %w", err)
	}
	defer func() { _ = f.Close() }()

	clipped := e
	clipped.StdoutTail = tail(e.StdoutTail, TailBytes)
	clipped.StderrTail = tail(e.StderrTail, TailBytes)
	if clipped.Timestamp.IsZero() {
		clipped.Timestamp = time.Now().UTC()
	}

	raw, err := json.Marshal(clipped)
	if err != nil {
		return fmt.Errorf("audit marshal: %w", err)
	}
	raw = append(raw, '\n')
	if _, err := f.Write(raw); err != nil {
		return fmt.Errorf("audit write: %w", err)
	}
	return nil
}

// tail returns the last n bytes of s. When s is shorter than n it
// is returned unchanged. When truncation happens the returned string
// is prefixed with "…(truncated)…" so a reader of the log can tell
// at a glance that there was more output.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…(truncated)…" + s[len(s)-n:]
}
