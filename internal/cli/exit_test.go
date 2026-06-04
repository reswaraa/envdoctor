// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/reswaraa/envdoctor/internal/output"
)

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name     string
		findings []output.Finding
		want     int
	}{
		{
			name:     "empty",
			findings: nil,
			want:     ExitOK,
		},
		{
			name: "all repairable",
			findings: []output.Finding{
				{ID: "a", Probe: "x", Status: output.StatusFail, RecipeID: "r1", DocURL: "x"},
				{ID: "b", Probe: "y", Status: output.StatusFail, RecipeID: "r2", DocURL: "x"},
			},
			want: ExitRepairable,
		},
		{
			name: "missing recipe",
			findings: []output.Finding{
				{ID: "a", Probe: "x", Status: output.StatusFail, RecipeID: "r1", DocURL: "x"},
				{ID: "b", Probe: "y", Status: output.StatusFail, DocURL: "x"},
			},
			want: ExitNoRecipe,
		},
		{
			name: "probe failed",
			findings: []output.Finding{
				{ID: "a", Probe: "boom", Status: output.StatusProbeFailed, DocURL: "x"},
			},
			want: ExitNoRecipe,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &output.Report{Findings: c.findings}
			if got := ExitCodeFor(r); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}

func TestAsExitCode(t *testing.T) {
	t.Run("plain error", func(t *testing.T) {
		if _, ok := asExitCode(errors.New("oops")); ok {
			t.Error("plain error should not match ExitCoder")
		}
	})
	t.Run("exitErr direct", func(t *testing.T) {
		code, ok := asExitCode(&exitErr{code: ExitNoRecipe})
		if !ok || code != ExitNoRecipe {
			t.Errorf("got code=%d ok=%v; want %d true", code, ok, ExitNoRecipe)
		}
	})
	t.Run("exitErr wrapped", func(t *testing.T) {
		wrapped := fmt.Errorf("outer: %w", &exitErr{code: ExitCrashed, err: errors.New("inner")})
		code, ok := asExitCode(wrapped)
		if !ok || code != ExitCrashed {
			t.Errorf("got code=%d ok=%v; want %d true", code, ok, ExitCrashed)
		}
	})
}
