// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/reswaraa/envdoctor/internal/probes"
	"github.com/reswaraa/envdoctor/internal/recipes"
)

// BuiltinProbes returns the slice of probes that ship with envdoctor.
//
// lib is threaded into each probe so Findings can be enriched with
// RecipeID and RecipeCommand from the bundled YAML library. A nil lib
// is allowed; probes degrade gracefully and emit Findings without
// recipes.
//
// New probes register here. The slice's order is not load-bearing
// (the engine sorts findings by probe ID at output time), but keeping
// it stable makes diffs easier to read.
func BuiltinProbes(lib *recipes.Library) []probes.Probe {
	return []probes.Probe{
		probes.NodeVersion(lib),
		probes.EnvRequired(lib),
		probes.PortFree(lib),
		probes.DockerRunning(lib),
		probes.PathCommand(lib),
		probes.ArchMismatch(lib),
	}
}
