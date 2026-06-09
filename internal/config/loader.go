// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/reswaraa/envdoctor/internal/semver"
)

// Stable error codes. These strings are part of the user-facing
// contract — third parties may grep CI output for them, so they
// cannot be renamed. Add new codes at the end of the list.
const (
	ErrYAMLParse                 = "E001"
	ErrMissingSchemaVersion      = "E002"
	ErrUnsupportedSchemaVersion  = "E003"
	ErrMinVersionUnparseable     = "E004"
	ErrMinVersionUnsatisfied     = "E005"
	ErrCheckMissingType          = "E006"
	ErrCheckUnknownType          = "E007"
	ErrCheckMissingRequiredField = "E008"
	ErrOverrideMissingID         = "E009"
	ErrDisableInvalidID          = "E010"
)

// Error is a stable-coded loader/validator error. It carries the
// schema field path so users can find the offending line quickly.
type Error struct {
	Code    string
	Message string
	Field   string
}

func (e *Error) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (at %s)", e.Code, e.Message, e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// FileName is the conventional name for the repo-level config.
const FileName = ".envdoctor.yaml"

// Load reads FileName from repoRoot, parses it, and validates against
// the schema. Returns (nil, nil) when the file is absent — the config
// is purely opt-in.
//
// envdoctorVersion is the running binary's semver. Passing "" or "dev"
// skips the min_version check so dev builds always load.
func Load(repoRoot, envdoctorVersion string) (*Config, error) {
	path := filepath.Join(repoRoot, FileName)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", FileName, err)
	}
	return LoadBytes(b, envdoctorVersion)
}

// LoadBytes parses and validates raw YAML bytes. Useful for tests and
// for the `envdoctor lint` reader of stdin.
func LoadBytes(b []byte, envdoctorVersion string) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, &Error{Code: ErrYAMLParse, Message: err.Error()}
	}
	if err := validate(&c); err != nil {
		return nil, err
	}
	if err := enforceMinVersion(&c, envdoctorVersion); err != nil {
		return nil, err
	}
	return &c, nil
}

func validate(c *Config) error {
	if c.SchemaVersion == 0 {
		return &Error{
			Code:    ErrMissingSchemaVersion,
			Message: "schema_version is required",
			Field:   "schema_version",
		}
	}
	if c.SchemaVersion != CurrentSchemaVersion {
		return &Error{
			Code: ErrUnsupportedSchemaVersion,
			Message: fmt.Sprintf("schema_version %d is not supported; this envdoctor reads schema_version %d",
				c.SchemaVersion, CurrentSchemaVersion),
			Field: "schema_version",
		}
	}
	for i, ck := range c.Checks {
		if err := validateCheck(i, ck); err != nil {
			return err
		}
	}
	for i, ov := range c.Overrides {
		if strings.TrimSpace(ov.ID) == "" {
			return &Error{
				Code:    ErrOverrideMissingID,
				Message: "override missing required field 'id'",
				Field:   fmt.Sprintf("overrides[%d].id", i),
			}
		}
	}
	for i, did := range c.Disable {
		if strings.TrimSpace(did) == "" {
			return &Error{
				Code:    ErrDisableInvalidID,
				Message: "disable entry must not be empty",
				Field:   fmt.Sprintf("disable[%d]", i),
			}
		}
	}
	return nil
}

func validateCheck(i int, ck Check) error {
	field := func(name string) string { return fmt.Sprintf("checks[%d].%s", i, name) }

	if ck.Type == "" {
		return &Error{
			Code:    ErrCheckMissingType,
			Message: "check missing required field 'type'",
			Field:   field("type"),
		}
	}

	switch ck.Type {
	case CheckToolVersion:
		if ck.Tool == "" {
			return &Error{Code: ErrCheckMissingRequiredField, Message: "tool_version requires 'tool'", Field: field("tool")}
		}
		if ck.Version == "" {
			return &Error{Code: ErrCheckMissingRequiredField, Message: "tool_version requires 'version'", Field: field("version")}
		}
	case CheckPortFree:
		if ck.Port <= 0 || ck.Port > 65535 {
			return &Error{Code: ErrCheckMissingRequiredField, Message: "port_free requires 'port' (1-65535)", Field: field("port")}
		}
	case CheckEnvRequired:
		if len(ck.Keys) == 0 {
			return &Error{Code: ErrCheckMissingRequiredField, Message: "env_required requires non-empty 'keys'", Field: field("keys")}
		}
	case CheckCommandPresent:
		if ck.Command == "" {
			return &Error{Code: ErrCheckMissingRequiredField, Message: "command_present requires 'command'", Field: field("command")}
		}
	default:
		return &Error{
			Code:    ErrCheckUnknownType,
			Message: fmt.Sprintf("unknown check type %q", ck.Type),
			Field:   field("type"),
		}
	}
	return nil
}

// enforceMinVersion compares the running envdoctor binary's semver
// against config.Envdoctor.MinVersion. "dev" / "" envdoctorVersion is
// treated as "always satisfies" so dev builds load any config.
func enforceMinVersion(c *Config, envdoctorVersion string) error {
	minVer := strings.TrimSpace(c.Envdoctor.MinVersion)
	if minVer == "" {
		return nil
	}
	if envdoctorVersion == "" || envdoctorVersion == "dev" {
		return nil
	}
	ok, err := semver.Satisfies(envdoctorVersion, ">="+minVer)
	if err != nil {
		return &Error{
			Code:    ErrMinVersionUnparseable,
			Message: fmt.Sprintf("envdoctor.min_version=%q is not a valid semver", minVer),
			Field:   "envdoctor.min_version",
		}
	}
	if !ok {
		return &Error{
			Code: ErrMinVersionUnsatisfied,
			Message: fmt.Sprintf("envdoctor %s is older than required min_version %s; run `envdoctor upgrade`",
				envdoctorVersion, minVer),
			Field: "envdoctor.min_version",
		}
	}
	return nil
}
