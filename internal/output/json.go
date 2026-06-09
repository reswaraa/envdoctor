// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package output defines the canonical Report data model emitted by
// envdoctor. The JSON shape produced by this package is the authoritative
// output of envdoctor: the --json flag, the debug bundle, and the future
// `envdoctor explain` reader all consume it. The TTY renderer in tty.go
// is a pure presentation layer over the same model.
package output

import (
	"encoding/json"
	"io"
	"time"
)

// SchemaVersion is the current Report schema version.
//
// Bumped only at *incompatible* JSON changes (a removed or renamed field,
// a changed semantic for an existing field). Adding new optional fields is
// not incompatible. The current major must continue to read the previous
// schema_version for one release window.
const SchemaVersion = "1"

// Report is the top-level envdoctor scan output.
//
// Findings is always a non-nil slice so consumers see "findings": [] in JSON
// rather than "findings": null. Engine output goes through NewReport which
// initializes the slice.
type Report struct {
	SchemaVersion    string    `json:"schema_version"`
	EnvdoctorVersion string    `json:"envdoctor_version"`
	RepoRoot         string    `json:"repo_root"`
	StartedAt        time.Time `json:"started_at"`
	FinishedAt       time.Time `json:"finished_at"`
	System           System    `json:"system"`
	Findings         []Finding `json:"findings"`
}

// System captures the facts about the user's machine at scan time.
type System struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Distro string `json:"distro,omitempty"`
	Kernel string `json:"kernel,omitempty"`
	Shell  string `json:"shell,omitempty"`
	WSL    bool   `json:"wsl"`
}

// Severity / Status / Category string values below are part of the canonical
// JSON schema. External consumers (CI scripts, debug bundles, future
// dashboards) read these strings. Their values cannot be renamed; the set
// can only be extended by adding new values.

// Severity is how serious a finding is.
type Severity string

// Severity values emitted in JSON output. Their string values are part
// of the canonical schema and cannot be renamed (see SchemaVersion).
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Status is the outcome of running a probe.
type Status string

// Status values emitted in JSON output. Their string values are part of
// the canonical schema and cannot be renamed (see SchemaVersion).
const (
	StatusOK          Status = "ok"
	StatusFail        Status = "fail"
	StatusSkipped     Status = "skipped"
	StatusProbeFailed Status = "probe_failed"
)

// Predefined categories used by built-in probes. Declarative `.envdoctor.yaml`
// checks use CategoryCustom unless they map onto an existing one.
const (
	CategoryRuntime      = "runtime"
	CategoryEnvironment  = "environment"
	CategoryDocker       = "docker"
	CategoryPorts        = "ports"
	CategoryPath         = "path"
	CategoryArchitecture = "architecture"
	CategoryCustom       = "custom"
)

// Finding is a single result emitted by a probe.
//
// DocURL is required; CI in Phase 8 fails the build if any emitted DocURL
// 404s on the docs site. RecipeID/RecipeCommand/RecipeClass are populated
// together when the recipe library has a fix for this finding; absent
// recipes are not an error, they signal that envdoctor needs a new recipe
// (exit code 2). RecipeClass is one of safe/shared/destructive/privileged
// — `envdoctor fix` uses it to drive the consent prompt and to refuse
// auto-running privileged commands.
type Finding struct {
	ID            string   `json:"id"`
	Probe         string   `json:"probe"`
	Category      string   `json:"category"`
	Severity      Severity `json:"severity"`
	Status        Status   `json:"status"`
	Summary       string   `json:"summary"`
	Observed      string   `json:"observed,omitempty"`
	Expected      string   `json:"expected,omitempty"`
	Evidence      []string `json:"evidence,omitempty"`
	RecipeID      string   `json:"recipe_id,omitempty"`
	RecipeClass   string   `json:"recipe_class,omitempty"`
	RecipeCommand string   `json:"recipe_command,omitempty"`
	DocURL        string   `json:"doc_url"`
}

// NewReport returns a Report initialized with the schema version, the
// envdoctor build version, the repo root, the system facts, and StartedAt
// set to now (UTC). Findings is a non-nil empty slice. Callers invoke
// Finalize when scanning completes.
func NewReport(envdoctorVersion, repoRoot string, sys System) *Report {
	return &Report{
		SchemaVersion:    SchemaVersion,
		EnvdoctorVersion: envdoctorVersion,
		RepoRoot:         repoRoot,
		StartedAt:        time.Now().UTC(),
		System:           sys,
		Findings:         []Finding{},
	}
}

// Finalize records FinishedAt as the current UTC time. Call once.
func (r *Report) Finalize() {
	r.FinishedAt = time.Now().UTC()
}

// WriteJSON writes r to w as pretty-indented JSON with a trailing newline.
// Pretty by default so a debug bundle can be read with `cat bundle.json |
// less`; JSON consumers (jq, scripts) handle indented JSON without issue.
func WriteJSON(w io.Writer, r *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
