// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogPath_XDGTakesPrecedenceOverHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/var/lib/state")
	t.Setenv("HOME", "/home/alice")
	got := LogPath()
	want := "/var/lib/state/envdoctor/audit.log"
	if got != want {
		t.Errorf("LogPath: got %q, want %q", got, want)
	}
}

func TestLogPath_FallsBackToHomeLocalState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/alice")
	got := LogPath()
	want := "/home/alice/.local/state/envdoctor/audit.log"
	if got != want {
		t.Errorf("LogPath: got %q, want %q", got, want)
	}
}

func TestLogPath_EmptyWhenNoHomeAndNoXDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")
	if got := LogPath(); got != "" {
		t.Errorf("LogPath without HOME/XDG should be empty; got %q", got)
	}
}

func TestAppendTo_CreatesParentDirectoryAndWritesEntry(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "envdoctor", "audit.log")

	ts := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	e := Entry{
		Timestamp:     ts,
		Command:       "brew install node@20",
		ExitCode:      0,
		StdoutTail:    "node@20 installed",
		RecipeID:      "brew-install-node",
		RecipeClass:   "shared",
		RecipeVersion: "ba3175a78a47",
	}
	if err := AppendTo(p, e); err != nil {
		t.Fatalf("AppendTo: %v", err)
	}

	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got Entry
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Command != e.Command {
		t.Errorf("Command: got %q, want %q", got.Command, e.Command)
	}
	if got.RecipeClass != "shared" {
		t.Errorf("RecipeClass: got %q, want %q", got.RecipeClass, "shared")
	}
	if !got.Timestamp.Equal(ts) {
		t.Errorf("Timestamp: got %v, want %v", got.Timestamp, ts)
	}
}

func TestAppendTo_PermissionsAre0600(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "envdoctor", "audit.log")
	if err := AppendTo(p, Entry{Command: "echo hi", ExitCode: 0}); err != nil {
		t.Fatalf("AppendTo: %v", err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// On a few corner-case filesystems (umask-aware tmpfs in CI) the
	// mode may include sticky/group bits. Assert the user-bits only.
	if got := fi.Mode().Perm() & 0o077; got != 0 {
		t.Errorf("audit log must be user-only readable; got mode %o", fi.Mode().Perm())
	}
}

func TestAppendTo_AppendsRatherThanOverwrites(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "audit.log")

	for i := range 3 {
		if err := AppendTo(p, Entry{Command: "cmd", ExitCode: i}); err != nil {
			t.Fatalf("AppendTo iter %d: %v", i, err)
		}
	}

	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	lines := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		lines++
		var e Entry
		if err := json.Unmarshal(s.Bytes(), &e); err != nil {
			t.Errorf("line %d not valid JSON: %v", lines, err)
		}
	}
	if lines != 3 {
		t.Errorf("expected 3 JSONL entries; got %d", lines)
	}
}

func TestAppendTo_StdoutTailIsTruncatedWithMarker(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "audit.log")

	big := strings.Repeat("x", TailBytes*2)
	if err := AppendTo(p, Entry{Command: "noisy", ExitCode: 0, StdoutTail: big}); err != nil {
		t.Fatalf("AppendTo: %v", err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var got Entry
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	// Truncation marker tells log readers there was more output.
	if !strings.HasPrefix(got.StdoutTail, "…(truncated)…") {
		t.Errorf("truncated stdout should be prefixed with marker; got first 40: %q", got.StdoutTail[:40])
	}
	// And the kept portion must be exactly the last TailBytes bytes.
	tailOnly := strings.TrimPrefix(got.StdoutTail, "…(truncated)…")
	if len(tailOnly) != TailBytes {
		t.Errorf("kept portion length: got %d, want %d", len(tailOnly), TailBytes)
	}
}

func TestAppendTo_EmptyPathIsNoOp(t *testing.T) {
	// LogPath() returns "" when no $HOME / $XDG; Append delegates to
	// AppendTo with "" — a silent drop. Must not error or panic.
	if err := AppendTo("", Entry{Command: "x"}); err != nil {
		t.Errorf("AppendTo(\"\"): expected nil error, got %v", err)
	}
}

func TestAppendTo_AutoSetsTimestampWhenZero(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "audit.log")
	before := time.Now().UTC()
	if err := AppendTo(p, Entry{Command: "x"}); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC()
	raw, _ := os.ReadFile(p)
	var got Entry
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Timestamp.Before(before) || got.Timestamp.After(after) {
		t.Errorf("auto-timestamp should be in [%v, %v]; got %v", before, after, got.Timestamp)
	}
}

func TestEntry_FieldNamesStable(t *testing.T) {
	e := Entry{
		Timestamp:     time.Unix(0, 0).UTC(),
		Command:       "x",
		ExitCode:      0,
		StdoutTail:    "out",
		StderrTail:    "err",
		RecipeID:      "rid",
		RecipeClass:   "safe",
		RecipeVersion: "v",
	}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"ts":`,
		`"command":`,
		`"exit_code":`,
		`"stdout_tail":`,
		`"stderr_tail":`,
		`"recipe_id":`,
		`"recipe_class":`,
		`"recipe_version":`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("audit field name drift: missing %q in %s", want, raw)
		}
	}
}
