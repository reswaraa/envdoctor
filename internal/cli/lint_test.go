// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runLintInDir cd's to dir, runs `lint`, and returns (stdout, stderr,
// exit code, error). cwd is restored on test cleanup so parallel tests
// don't collide. The command is wired through cobra to exercise the
// real RunE path.
func runLintInDir(t *testing.T, dir string) (stdout, stderr string, code int) {
	t.Helper()

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	cmd := newLintCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(nil)

	err = cmd.Execute()
	code = ExitOK
	if err != nil {
		var ec *exitErr
		if errors.As(err, &ec) {
			code = ec.code
		} else {
			code = ExitCrashed
		}
		// Cobra prints errors unless silenced; we explicitly silenced
		// on the root, but newLintCmd here has SilenceErrors=false by
		// default. The test asserts on the captured stderr from RunE,
		// not from cobra's own error handling.
		_ = err
	}
	return outBuf.String(), errBuf.String(), code
}

func TestLint_NoConfigIsOK(t *testing.T) {
	stdout, _, code := runLintInDir(t, t.TempDir())
	if code != ExitOK {
		t.Errorf("exit code: got %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stdout, "nothing to lint") {
		t.Errorf("stdout should explain absence; got %q", stdout)
	}
}

func TestLint_ValidConfigIsOK(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`schema_version: 1
checks:
  - type: tool_version
    tool: psql
    version: ">=14"
`)
	if err := os.WriteFile(filepath.Join(dir, ".envdoctor.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, code := runLintInDir(t, dir)
	if code != ExitOK {
		t.Errorf("exit code: got %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stdout, "ok") {
		t.Errorf("stdout should start with 'ok'; got %q", stdout)
	}
	if !strings.Contains(stdout, "1 check") {
		t.Errorf("stdout should report check count; got %q", stdout)
	}
}

func TestLint_MalformedConfigExits4(t *testing.T) {
	dir := t.TempDir()
	body := []byte("schema_version: 1\nchecks:\n  - type: not_a_known_type\n")
	if err := os.WriteFile(filepath.Join(dir, ".envdoctor.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runLintInDir(t, dir)
	if code != ExitConfigParseError {
		t.Errorf("exit code: got %d, want %d", code, ExitConfigParseError)
	}
	if !strings.Contains(stderr, "E007") {
		t.Errorf("stderr should include the stable error code; got %q", stderr)
	}
}

func TestLint_MissingSchemaVersionEmitsE002(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".envdoctor.yaml"), []byte("checks: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runLintInDir(t, dir)
	if code != ExitConfigParseError {
		t.Errorf("exit code: %d", code)
	}
	if !strings.Contains(stderr, "E002") {
		t.Errorf("expected E002 in stderr; got %q", stderr)
	}
}
