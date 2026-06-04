// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package probes defines the Probe contract every diagnostic check
// implements. Implementations live in their own files under this package
// (one Probe per file). The engine fans them out in parallel; see
// internal/engine for the runner.
//
// Probe IDs are stable forever. Renaming a probe ID breaks every debug
// bundle and every external Finding.doc_url. Deprecate-and-add instead:
// keep the old probe returning a warning Finding that points at the new
// probe and the new doc page.
package probes

import (
	"context"

	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/system"
)

// Input is the per-scan context the engine passes to each probe.
type Input struct {
	// RepoRoot is the absolute path to the directory being scanned.
	RepoRoot string

	// System holds the collected machine facts plus a shared HasTool cache.
	// Probes may call System.HasTool concurrently — the cache is
	// mutex-guarded so parallel access is safe.
	System *system.Facts
}

// Probe is the contract every check implements.
//
// A Probe's single responsibility is to compare some aspect of the local
// environment against what the repo needs and emit zero or more Findings.
//
// AppliesTo is the cheap pre-flight check: probes that don't apply at all
// (e.g. the Node version probe in a Go-only repo) return false and are
// omitted from the scan entirely. AppliesTo must not exec subprocesses.
//
// Run executes the work. Run respects ctx cancellation (Ctrl-C); any
// long-running subprocess must use exec.CommandContext or poll ctx.Done().
// Run may return ([]Finding, nil) — including an empty slice for "all
// clear" — or (nil, error) when the probe itself cannot run. The engine
// recovers panics and turns returned errors into a single Finding with
// Status = StatusProbeFailed.
type Probe interface {
	ID() string
	AppliesTo(in Input) bool
	Run(ctx context.Context, in Input) ([]output.Finding, error)
}
