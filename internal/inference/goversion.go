// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// GoRequirement is the Go toolchain version constraint from go.mod.
// go.mod's `go X.Y` directive means "needs at least X.Y", but we
// treat it as an exact target for envdoctor purposes — most version
// managers install the specified version.
type GoRequirement struct {
	Source     string
	Constraint string
	IsExact    bool
}

// AsConstraint expands Go's `go X.Y` short form into ">=X.Y.0", since
// go.mod's directive is documented as a minimum, not an exact pin.
//
//	"1.21"     -> ">=1.21.0"
//	"1.21.5"   -> ">=1.21.5"
//	"^1.21"    -> "^1.21" (passthrough)
func (r GoRequirement) AsConstraint() string {
	raw := strings.TrimSpace(r.Constraint)
	if hasConstraintOperator(raw) {
		return raw
	}
	parts := strings.Split(raw, ".")
	switch len(parts) {
	case 1:
		return ">=" + raw + ".0.0"
	case 2:
		return ">=" + raw + ".0"
	default:
		return ">=" + raw
	}
}

// InferGo extracts the Go directive from go.mod.
func InferGo(root string) ([]GoRequirement, error) {
	p := filepath.Join(root, "go.mod")
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "go ") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(line, "go "))
		// Drop trailing comments, e.g. `go 1.21 // toolchain hint`
		if i := strings.Index(v, "//"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		if v == "" {
			continue
		}
		return []GoRequirement{{Source: "go.mod", Constraint: v, IsExact: false}}, nil
	}
	return nil, nil
}
