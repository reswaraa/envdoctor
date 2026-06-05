// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package recipes loads, validates, and matches the YAML recipe library
// that ships embedded in the binary. The recipe library is the
// defensibility play: a large, well-tested set of repair commands for
// every (probe, os, tool) combination an envdoctor user is likely to
// hit.
package recipes

// Recipe is one logical group of repair commands addressing the same
// kind of finding (e.g. "node version mismatch"). The matcher selects
// at most one Fix per Finding based on system facts.
type Recipe struct {
	ID    string `yaml:"id"`
	Probe string `yaml:"probe"`
	Fixes []Fix  `yaml:"fixes"`
}

// Fix is one concrete command, scoped by When and classified by Class.
//
// Command is a text/template expression (Phase 2A matcher) with
// finding-supplied parameters: e.g. "mise install node@{{.Required}}".
// Fallback marks a Fix as a last resort — used only when no other Fix
// in the same Recipe matches the system.
type Fix struct {
	ID       string `yaml:"id"`
	When     Match  `yaml:"when"`
	Class    Class  `yaml:"class"`
	Label    string `yaml:"label,omitempty"`
	Command  string `yaml:"command"`
	Fallback bool   `yaml:"fallback,omitempty"`
	Test     Test   `yaml:"test,omitempty"`
}

// Class is the safety category controlling `--fix` behavior (Phase 6).
type Class string

// Class values. Strings ship in JSON output and in the YAML schema;
// renaming any of them is an incompatible recipe-format change.
const (
	ClassSafe        Class = "safe"
	ClassShared      Class = "shared"
	ClassDestructive Class = "destructive"
	ClassPrivileged  Class = "privileged"
)

// Match is a Fix's selection clause. The Fix applies when every
// non-empty field equals the corresponding system fact (HasTool is
// checked via Facts.HasTool). Empty fields are wildcards.
type Match struct {
	OS      string `yaml:"os,omitempty"`
	Arch    string `yaml:"arch,omitempty"`
	Distro  string `yaml:"distro,omitempty"`
	HasTool string `yaml:"has_tool,omitempty"`
}

// Test is the before/after fixture for the Phase 2C recipe contract
// harness. Each Fix ships with one; CI runs the recipe twice in a
// fresh container per Fix and asserts idempotence.
type Test struct {
	Image  string            `yaml:"image,omitempty"`
	Before TestStep          `yaml:"before,omitempty"`
	After  TestStep          `yaml:"after,omitempty"`
	Params map[string]string `yaml:"params,omitempty"`
}

// TestStep is one half of a recipe test: a shell command whose exit
// code says whether the precondition or postcondition holds.
type TestStep struct {
	Check      string `yaml:"check,omitempty"`
	Idempotent bool   `yaml:"idempotent,omitempty"`
}
