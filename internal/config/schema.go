// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package config parses, validates, and represents the repo-level
// .envdoctor.yaml file. The schema uses a typed-list discriminator:
// every Check carries a `type:` field that determines which other
// fields are meaningful.
//
// The authoritative JSON Schema for editor / IDE integration lives at
// docs/schema/v1/config.json and is published at
// https://reswaraa.github.io/envdoctor/schemas/v1/config.json. Users can add this
// header to their .envdoctor.yaml for YAML-language-server hints:
//
//	# yaml-language-server: $schema=https://reswaraa.github.io/envdoctor/schemas/v1/config.json
//
// Adding a new check `type:` is a *minor* schema change (additive);
// removing or renaming an existing field is *major* and bumps the
// SchemaVersion constant.
package config

// CurrentSchemaVersion is the schema version the loader accepts.
// Existing configs with this version load cleanly; newer versions
// are rejected. When bumped, a parallel-supported fallback must be
// added to LoadBytes so older configs remain readable for one release.
const CurrentSchemaVersion = 1

// Config is the parsed `.envdoctor.yaml` document.
type Config struct {
	SchemaVersion int            `yaml:"schema_version"`
	Envdoctor     EnvdoctorBlock `yaml:"envdoctor,omitempty"`
	Checks        []Check        `yaml:"checks,omitempty"`
	Overrides     []Override     `yaml:"overrides,omitempty"`
	Disable       []string       `yaml:"disable,omitempty"`
}

// EnvdoctorBlock pins meta-fields like the minimum envdoctor binary
// version expected to read this config.
type EnvdoctorBlock struct {
	MinVersion string `yaml:"min_version,omitempty"`
}

// Known check `type:` strings. New types append to this list; the
// strings are forever (part of the YAML schema contract).
const (
	CheckToolVersion    = "tool_version"
	CheckPortFree       = "port_free"
	CheckEnvRequired    = "env_required"
	CheckCommandPresent = "command_present"
)

// Check is one declarative additive check. `Type` is the discriminator;
// the other fields are populated per-type. Validation in loader.go
// rejects checks whose type-specific required fields are missing.
type Check struct {
	Type   string `yaml:"type"`
	Reason string `yaml:"reason,omitempty"`

	// tool_version
	Tool    string `yaml:"tool,omitempty"`
	Version string `yaml:"version,omitempty"`

	// port_free
	Port int `yaml:"port,omitempty"`

	// env_required
	File string   `yaml:"file,omitempty"`
	Keys []string `yaml:"keys,omitempty"`

	// command_present
	Command string `yaml:"command,omitempty"`
}

// Override modifies an inferred check parameter without disabling
// inference entirely. ID refers to a stable inferred check identifier
// of the form "inferred:<probe>:<source>" or
// "inferred:<probe>:<source>#<key>".
type Override struct {
	ID      string `yaml:"id"`
	Version string `yaml:"version,omitempty"`
	Port    int    `yaml:"port,omitempty"`
}
