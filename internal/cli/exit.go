// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"

	"github.com/reswaraa/envdoctor/internal/output"
)

// Process exit codes.
const (
	ExitOK               = 0
	ExitRepairable       = 1
	ExitNoRecipe         = 2
	ExitCrashed          = 3
	ExitConfigParseError = 4
)

// exitErr is the cobra RunE → Execute transport for an intended process
// exit code. err is optional: codes that already paired with user-visible
// output (e.g. a finished scan that produced findings) pass err=nil so
// Execute does not double-print.
type exitErr struct {
	code int
	err  error
}

func (e *exitErr) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return ""
}

func (e *exitErr) Unwrap() error { return e.err }
func (e *exitErr) ExitCode() int { return e.code }

type exitCoder interface {
	ExitCode() int
}

// asExitCode unwraps err and returns the embedded ExitCode if any.
func asExitCode(err error) (int, bool) {
	var ec exitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode(), true
	}
	return 0, false
}

// ExitCodeFor computes the exit code for a completed scan report.
//
//	ExitOK         no findings
//	ExitNoRecipe   any finding lacks RecipeID
//	ExitRepairable findings exist, all have a known recipe
//
// Findings with Status=StatusProbeFailed never carry a RecipeID, so a
// panicking probe yields ExitNoRecipe — the signal CI can read as
// "envdoctor needs a new recipe", distinct from ExitCrashed (the CLI
// itself failed).
func ExitCodeFor(r *output.Report) int {
	if len(r.Findings) == 0 {
		return ExitOK
	}
	for _, f := range r.Findings {
		if f.RecipeID == "" {
			return ExitNoRecipe
		}
	}
	return ExitRepairable
}
