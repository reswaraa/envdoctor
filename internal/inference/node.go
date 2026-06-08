// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package inference walks a repository root and extracts version /
// dependency / port requirements from standard manifest files.
// Probes consume Requirements; they don't read manifest files directly.
package inference

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// NodeRequirement is one constraint on the Node runtime version.
// Source is the manifest path relative to the repo root (or with a
// JSON-path-style suffix for sub-fields).
type NodeRequirement struct {
	Source     string
	Constraint string
	IsExact    bool
}

// AsConstraint returns Constraint expanded to npm-style semver, mirroring
// how version managers actually interpret short-form versions:
//
//	"20"      -> "^20.0.0"   (latest major 20)
//	"20.10"   -> "~20.10.0"  (latest minor 20.10)
//	"20.10.0" -> "20.10.0"   (exact)
//	"v20"     -> "^20.0.0"   ('v' prefix stripped)
//	"^20"     -> "^20"       (already a constraint expression; pass-through)
func (r NodeRequirement) AsConstraint() string {
	raw := strings.TrimSpace(r.Constraint)
	raw = strings.TrimPrefix(raw, "v")

	if hasConstraintOperator(raw) {
		return raw
	}
	parts := strings.Split(raw, ".")
	switch len(parts) {
	case 1:
		return "^" + raw + ".0.0"
	case 2:
		return "~" + raw + ".0"
	default:
		return raw
	}
}

func hasConstraintOperator(s string) bool {
	for _, ch := range s {
		switch ch {
		case '^', '~', '>', '<', '=', '*', ',', ' ', 'x', 'X', '|':
			return true
		}
	}
	return false
}

// InferNode collects Node version requirements from standard manifest
// files under root, in priority order:
//
//	.nvmrc, .node-version, .tool-versions, .mise.toml / mise.toml,
//	package.json#engines.node
//
// Returns a nil slice if no Node signals are found.
func InferNode(root string) ([]NodeRequirement, error) {
	var out []NodeRequirement

	for _, fn := range []func(string) (NodeRequirement, bool, error){
		readNVMRC,
		readNodeVersionFile,
		readToolVersionsNode,
		readMiseTOMLNode,
		readPackageJSONNodeEngines,
	} {
		r, ok, err := fn(root)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func readNVMRC(root string) (NodeRequirement, bool, error) {
	return readSingleLineFile(root, ".nvmrc")
}

func readNodeVersionFile(root string) (NodeRequirement, bool, error) {
	return readSingleLineFile(root, ".node-version")
}

// readSingleLineFile is the shared one-line-version-file reader for
// .nvmrc, .node-version, .python-version, .ruby-version. All these
// files contain a single exact version string by convention.
func readSingleLineFile(root, name string) (NodeRequirement, bool, error) {
	p := filepath.Join(root, name)
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return NodeRequirement{}, false, nil
		}
		return NodeRequirement{}, false, fmt.Errorf("read %s: %w", name, err)
	}
	v := strings.TrimSpace(string(b))
	if v == "" || strings.HasPrefix(v, "#") {
		return NodeRequirement{}, false, nil
	}
	return NodeRequirement{Source: name, Constraint: v, IsExact: true}, true, nil
}

func readToolVersionsNode(root string) (NodeRequirement, bool, error) {
	p := filepath.Join(root, ".tool-versions")
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return NodeRequirement{}, false, nil
		}
		return NodeRequirement{}, false, fmt.Errorf("read .tool-versions: %w", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "nodejs" || fields[0] == "node" {
			return NodeRequirement{Source: ".tool-versions", Constraint: fields[1], IsExact: true}, true, nil
		}
	}
	return NodeRequirement{}, false, nil
}

func readMiseTOMLNode(root string) (NodeRequirement, bool, error) {
	for _, name := range []string{".mise.toml", "mise.toml"} {
		p := filepath.Join(root, name)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		var doc struct {
			Tools map[string]any `toml:"tools"`
		}
		if _, err := toml.DecodeFile(p, &doc); err != nil {
			return NodeRequirement{}, false, fmt.Errorf("parse %s: %w", name, err)
		}
		v, ok := doc.Tools["node"]
		if !ok {
			v, ok = doc.Tools["nodejs"]
			if !ok {
				continue
			}
		}
		switch s := v.(type) {
		case string:
			return NodeRequirement{Source: name, Constraint: s, IsExact: true}, true, nil
		case map[string]any:
			if str, ok := s["version"].(string); ok {
				return NodeRequirement{Source: name, Constraint: str, IsExact: true}, true, nil
			}
		}
	}
	return NodeRequirement{}, false, nil
}

func readPackageJSONNodeEngines(root string) (NodeRequirement, bool, error) {
	p := filepath.Join(root, "package.json")
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return NodeRequirement{}, false, nil
		}
		return NodeRequirement{}, false, fmt.Errorf("read package.json: %w", err)
	}
	var pkg struct {
		Engines struct {
			Node string `json:"node"`
		} `json:"engines"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return NodeRequirement{}, false, fmt.Errorf("parse package.json: %w", err)
	}
	if strings.TrimSpace(pkg.Engines.Node) == "" {
		return NodeRequirement{}, false, nil
	}
	return NodeRequirement{
		Source:     "package.json#engines.node",
		Constraint: pkg.Engines.Node,
		IsExact:    false,
	}, true, nil
}
