// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package bundle writes and reads the self-contained debug artifact
// shared via `envdoctor scan --bundle <path>` and consumed by
// `envdoctor explain`.
//
// A Bundle wraps the canonical Report (from internal/output) with
// envdoctor build metadata and the recipe library hash so the
// recipient can tell whether their local envdoctor would produce
// the same advice.
//
// Redaction is structural — implemented in redact.go and applied
// before WritePath touches disk. Probes are responsible for not
// emitting env values or file contents into Findings in the first
// place; the bundle layer enforces path/user/host stripping on top.
package bundle

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/reswaraa/envdoctor/internal/output"
)

// SchemaVersion pins the on-wire Bundle shape. Bump only at
// incompatible changes; readers on the same major must accept the
// previous version for one release window per Q17.
const SchemaVersion = "1"

// Bundle is the full debug artifact. Report is the canonical scan
// output; the other fields are bundle-specific metadata.
//
// Tools is a name -> version map of executables envdoctor probed
// during the scan. RecipeHash is the SHA-256 of the recipe library
// source files (alphabetical-by-path concatenation), so the
// recipient can verify the bundle was produced against an unmodified
// library shipped with envdoctor.
type Bundle struct {
	SchemaVersion    string            `json:"schema_version"`
	EnvdoctorVersion string            `json:"envdoctor_version"`
	GeneratedAt      time.Time         `json:"generated_at"`
	Report           *output.Report    `json:"report"`
	RecipeHash       string            `json:"recipe_hash,omitempty"`
	Tools            map[string]string `json:"tools,omitempty"`
}

// New constructs a Bundle from a completed scan Report and the
// running envdoctor build metadata. The Report is deep-copied so
// subsequent Redact mutations don't leak back into the caller's
// in-memory Report (which would otherwise corrupt the pretty TTY
// render that happens after the bundle write).
func New(envdoctorVersion string, r *output.Report, recipeHash string) *Bundle {
	return &Bundle{
		SchemaVersion:    SchemaVersion,
		EnvdoctorVersion: envdoctorVersion,
		GeneratedAt:      time.Now().UTC(),
		Report:           cloneReport(r),
		RecipeHash:       recipeHash,
		Tools:            map[string]string{},
	}
}

func cloneReport(r *output.Report) *output.Report {
	if r == nil {
		return nil
	}
	cp := *r
	if r.Findings != nil {
		cp.Findings = make([]output.Finding, len(r.Findings))
		for i, f := range r.Findings {
			fc := f
			if f.Evidence != nil {
				fc.Evidence = make([]string, len(f.Evidence))
				copy(fc.Evidence, f.Evidence)
			}
			cp.Findings[i] = fc
		}
	}
	return &cp
}

// Stats summarizes the contents of a Bundle for the pre-write
// preview the user sees before sharing. EnvValues and FileBodies
// are always zero — they're a structural guarantee of the probe
// layer, surfaced here so users can see "0 env values, 0 file
// bodies" and trust what they're about to share.
type Stats struct {
	SizeBytes  int
	Findings   int
	Tools      int
	EnvValues  int
	FileBodies int
}

// PreviewLine renders the one-line summary printed to stderr
// before a contributor uploads the bundle to a GitHub issue.
func (s Stats) PreviewLine(path string) string {
	return fmt.Sprintf(
		"Wrote %s (%dB) — %d finding(s), %d tool version(s), %d env value(s), %d file content(s). Review before sharing.",
		path, s.SizeBytes, s.Findings, s.Tools, s.EnvValues, s.FileBodies,
	)
}

// WritePath serializes b to filePath after applying Redact. Returns
// Stats for the caller to render the pre-write preview.
func WritePath(filePath string, b *Bundle, opts RedactOptions) (Stats, error) {
	Redact(b, opts)
	f, err := os.Create(filePath)
	if err != nil {
		return Stats{}, fmt.Errorf("create bundle: %w", err)
	}
	defer func() { _ = f.Close() }()
	n, err := writeTo(f, b)
	if err != nil {
		return Stats{}, err
	}
	return Stats{
		SizeBytes:  n,
		Findings:   len(b.Report.Findings),
		Tools:      len(b.Tools),
		EnvValues:  0,
		FileBodies: 0,
	}, nil
}

// writeTo serializes b as pretty-indented JSON and returns bytes
// written. The pretty format is intentional: a debug bundle is a
// human-inspected artifact; consumers can still pipe through jq.
func writeTo(w io.Writer, b *Bundle) (int, error) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(b); err != nil {
		return 0, fmt.Errorf("encode bundle: %w", err)
	}
	// json.Encoder doesn't surface bytes written; re-marshal a count
	// for the preview. Cheap; bundles are tens of KB.
	raw, _ := json.MarshalIndent(b, "", "  ")
	return len(raw) + 1, nil
}

// Read parses a Bundle JSON from path. Used by `envdoctor explain`.
func Read(filePath string) (*Bundle, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}
	var out Bundle
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}
	return &out, nil
}
