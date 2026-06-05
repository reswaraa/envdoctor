// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package semver is the thin wrapper envdoctor probes use for version
// comparisons and constraint matching. It delegates to
// github.com/Masterminds/semver/v3 (npm-style constraint syntax:
// ^, ~, >=, <=, >, <, =, ranges) and centralizes a few normalization
// conventions:
//
//   - leading 'v' tolerated (`v18.17.0` and `18.17.0` are equivalent)
//   - one- and two-segment versions accepted (`20` -> `20.0.0`)
//   - leading/trailing whitespace stripped (manifests often have a
//     trailing newline)
package semver

import (
	"fmt"
	"strings"

	mm "github.com/Masterminds/semver/v3"
)

// Compare returns -1 if a < b, 0 if a == b, +1 if a > b.
func Compare(a, b string) (int, error) {
	va, err := parseVersion(a)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", a, err)
	}
	vb, err := parseVersion(b)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", b, err)
	}
	return va.Compare(vb), nil
}

// Satisfies reports whether version meets the constraint.
//
//	Satisfies("20.10.0", "^20.0.0") -> true
//	Satisfies("18.17.0", "^20.0.0") -> false
//	Satisfies("20.10.0", ">=18")    -> true
//	Satisfies("20.10.0", "20.x")    -> true
func Satisfies(version, constraint string) (bool, error) {
	v, err := parseVersion(version)
	if err != nil {
		return false, fmt.Errorf("parse version %q: %w", version, err)
	}
	c, err := mm.NewConstraint(strings.TrimSpace(constraint))
	if err != nil {
		return false, fmt.Errorf("parse constraint %q: %w", constraint, err)
	}
	return c.Check(v), nil
}

// Major returns the major version number of v. v may have a 'v' prefix
// and may be one-, two-, or three-segment.
func Major(v string) (int, error) {
	parsed, err := parseVersion(v)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", v, err)
	}
	return int(parsed.Major()), nil
}

func parseVersion(s string) (*mm.Version, error) {
	return mm.NewVersion(strings.TrimSpace(s))
}
