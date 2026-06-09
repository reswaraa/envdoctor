// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadBytes_ValidMinimal(t *testing.T) {
	c, err := LoadBytes([]byte("schema_version: 1\n"), "")
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if c.SchemaVersion != 1 {
		t.Errorf("SchemaVersion: got %d, want 1", c.SchemaVersion)
	}
}

func TestLoadBytes_ValidFull(t *testing.T) {
	src := `schema_version: 1
envdoctor:
  min_version: "0.1.0"

checks:
  - type: tool_version
    tool: postgres
    version: ">=14.0"
    reason: "we use jsonpath_exists"
  - type: port_free
    port: 5432
  - type: env_required
    file: .env
    keys: [DATABASE_URL, JWT_SECRET]
  - type: command_present
    command: make

overrides:
  - id: inferred:node-version:package.json#engines.node
    version: "~20.10.0"

disable:
  - inferred:port-free:docker-compose.yml#3000
`
	c, err := LoadBytes([]byte(src), "0.1.0")
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if len(c.Checks) != 4 {
		t.Errorf("Checks: got %d, want 4", len(c.Checks))
	}
	if len(c.Overrides) != 1 {
		t.Errorf("Overrides: got %d, want 1", len(c.Overrides))
	}
	if len(c.Disable) != 1 {
		t.Errorf("Disable: got %d, want 1", len(c.Disable))
	}
	if c.Envdoctor.MinVersion != "0.1.0" {
		t.Errorf("MinVersion: %q", c.Envdoctor.MinVersion)
	}
}

func TestLoadBytes_MalformedYAML(t *testing.T) {
	_, err := LoadBytes([]byte("not: valid: yaml: :::"), "")
	var e *Error
	if !errors.As(err, &e) || e.Code != ErrYAMLParse {
		t.Errorf("expected E001 YAML parse error; got %v", err)
	}
}

func TestLoadBytes_MissingSchemaVersion(t *testing.T) {
	_, err := LoadBytes([]byte("checks: []\n"), "")
	var e *Error
	if !errors.As(err, &e) || e.Code != ErrMissingSchemaVersion {
		t.Errorf("expected E002; got %v", err)
	}
}

func TestLoadBytes_UnsupportedSchemaVersion(t *testing.T) {
	_, err := LoadBytes([]byte("schema_version: 99\n"), "")
	var e *Error
	if !errors.As(err, &e) || e.Code != ErrUnsupportedSchemaVersion {
		t.Errorf("expected E003; got %v", err)
	}
}

func TestLoadBytes_MinVersionUnsatisfied(t *testing.T) {
	src := "schema_version: 1\nenvdoctor:\n  min_version: \"2.0.0\"\n"
	_, err := LoadBytes([]byte(src), "1.0.0")
	var e *Error
	if !errors.As(err, &e) || e.Code != ErrMinVersionUnsatisfied {
		t.Errorf("expected E005; got %v", err)
	}
}

func TestLoadBytes_MinVersionSkippedForDev(t *testing.T) {
	src := "schema_version: 1\nenvdoctor:\n  min_version: \"99.0.0\"\n"
	if _, err := LoadBytes([]byte(src), "dev"); err != nil {
		t.Errorf("dev build should not be blocked by min_version; got %v", err)
	}
	if _, err := LoadBytes([]byte(src), ""); err != nil {
		t.Errorf("empty version should not be blocked by min_version; got %v", err)
	}
}

func TestLoadBytes_MinVersionUnparseable(t *testing.T) {
	src := "schema_version: 1\nenvdoctor:\n  min_version: \"not-a-version\"\n"
	_, err := LoadBytes([]byte(src), "1.0.0")
	var e *Error
	if !errors.As(err, &e) || e.Code != ErrMinVersionUnparseable {
		t.Errorf("expected E004; got %v", err)
	}
}

func TestLoadBytes_CheckMissingType(t *testing.T) {
	src := "schema_version: 1\nchecks:\n  - reason: nope\n"
	_, err := LoadBytes([]byte(src), "")
	var e *Error
	if !errors.As(err, &e) || e.Code != ErrCheckMissingType {
		t.Errorf("expected E006; got %v", err)
	}
}

func TestLoadBytes_CheckUnknownType(t *testing.T) {
	src := "schema_version: 1\nchecks:\n  - type: not_a_known_type\n"
	_, err := LoadBytes([]byte(src), "")
	var e *Error
	if !errors.As(err, &e) || e.Code != ErrCheckUnknownType {
		t.Errorf("expected E007; got %v", err)
	}
	if !strings.Contains(e.Message, "not_a_known_type") {
		t.Errorf("message should quote the bad type; got %v", e)
	}
}

func TestLoadBytes_CheckMissingRequiredField(t *testing.T) {
	cases := []struct {
		name, body string
	}{
		{"tool_version missing tool", "schema_version: 1\nchecks:\n  - type: tool_version\n    version: \">=1\"\n"},
		{"tool_version missing version", "schema_version: 1\nchecks:\n  - type: tool_version\n    tool: psql\n"},
		{"port_free missing port", "schema_version: 1\nchecks:\n  - type: port_free\n"},
		{"port_free port out of range", "schema_version: 1\nchecks:\n  - type: port_free\n    port: 99999\n"},
		{"env_required no keys", "schema_version: 1\nchecks:\n  - type: env_required\n    file: .env\n"},
		{"command_present missing command", "schema_version: 1\nchecks:\n  - type: command_present\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := LoadBytes([]byte(c.body), "")
			var e *Error
			if !errors.As(err, &e) || e.Code != ErrCheckMissingRequiredField {
				t.Errorf("expected E008; got %v", err)
			}
		})
	}
}

func TestLoadBytes_OverrideMissingID(t *testing.T) {
	src := "schema_version: 1\noverrides:\n  - version: '20.10.0'\n"
	_, err := LoadBytes([]byte(src), "")
	var e *Error
	if !errors.As(err, &e) || e.Code != ErrOverrideMissingID {
		t.Errorf("expected E009; got %v", err)
	}
}

func TestLoadBytes_DisableInvalidID(t *testing.T) {
	src := "schema_version: 1\ndisable:\n  - \"\"\n"
	_, err := LoadBytes([]byte(src), "")
	var e *Error
	if !errors.As(err, &e) || e.Code != ErrDisableInvalidID {
		t.Errorf("expected E010; got %v", err)
	}
}

func TestLoad_FileMissingReturnsNilNoError(t *testing.T) {
	c, err := Load(t.TempDir(), "")
	if err != nil {
		t.Errorf("missing file must not be an error; got %v", err)
	}
	if c != nil {
		t.Errorf("missing file must return nil config; got %+v", c)
	}
}

func TestLoad_ReadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c == nil || c.SchemaVersion != 1 {
		t.Errorf("Load: got %+v", c)
	}
}
