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

	"github.com/BurntSushi/toml"
)

// PythonRequirement is one constraint on the Python runtime version.
type PythonRequirement struct {
	Source     string
	Constraint string
	IsExact    bool
}

// AsConstraint returns Constraint in npm-style semver. Short forms
// follow PEP-440-ish conventions as interpreted by version managers:
//
//	"3.11"   -> "~3.11.0"   (latest 3.11 patch)
//	"3"      -> "^3.0.0"
//	"3.11.5" -> "3.11.5"
//	"^3.10"  -> "^3.10"     (already an operator; passthrough)
func (r PythonRequirement) AsConstraint() string {
	raw := strings.TrimSpace(r.Constraint)
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

// InferPython collects Python version requirements from:
//
//	.python-version, .tool-versions, .mise.toml / mise.toml,
//	pyproject.toml#project.requires-python,
//	pyproject.toml#tool.poetry.dependencies.python
//
// Returns a nil slice if no signals are found.
func InferPython(root string) ([]PythonRequirement, error) {
	var out []PythonRequirement
	for _, fn := range []func(string) (PythonRequirement, bool, error){
		readPythonVersionFile,
		readToolVersionsPython,
		readMiseTOMLPython,
		readPyprojectPython,
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

func readPythonVersionFile(root string) (PythonRequirement, bool, error) {
	r, ok, err := readSingleLineFile(root, ".python-version")
	if err != nil || !ok {
		return PythonRequirement{}, ok, err
	}
	return PythonRequirement(r), true, nil
}

func readToolVersionsPython(root string) (PythonRequirement, bool, error) {
	p := filepath.Join(root, ".tool-versions")
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return PythonRequirement{}, false, nil
		}
		return PythonRequirement{}, false, fmt.Errorf("read .tool-versions: %w", err)
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
		if fields[0] == "python" {
			return PythonRequirement{Source: ".tool-versions", Constraint: fields[1], IsExact: true}, true, nil
		}
	}
	return PythonRequirement{}, false, nil
}

func readMiseTOMLPython(root string) (PythonRequirement, bool, error) {
	for _, name := range []string{".mise.toml", "mise.toml"} {
		p := filepath.Join(root, name)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		var doc struct {
			Tools map[string]any `toml:"tools"`
		}
		if _, err := toml.DecodeFile(p, &doc); err != nil {
			return PythonRequirement{}, false, fmt.Errorf("parse %s: %w", name, err)
		}
		v, ok := doc.Tools["python"]
		if !ok {
			continue
		}
		switch s := v.(type) {
		case string:
			return PythonRequirement{Source: name, Constraint: s, IsExact: true}, true, nil
		case map[string]any:
			if str, ok := s["version"].(string); ok {
				return PythonRequirement{Source: name, Constraint: str, IsExact: true}, true, nil
			}
		}
	}
	return PythonRequirement{}, false, nil
}

func readPyprojectPython(root string) (PythonRequirement, bool, error) {
	p := filepath.Join(root, "pyproject.toml")
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return PythonRequirement{}, false, nil
		}
		return PythonRequirement{}, false, fmt.Errorf("read pyproject.toml: %w", err)
	}
	var doc struct {
		Project struct {
			RequiresPython string `toml:"requires-python"`
		} `toml:"project"`
		Tool struct {
			Poetry struct {
				Dependencies map[string]any `toml:"dependencies"`
			} `toml:"poetry"`
		} `toml:"tool"`
	}
	if err := toml.Unmarshal(b, &doc); err != nil {
		return PythonRequirement{}, false, fmt.Errorf("parse pyproject.toml: %w", err)
	}
	if v := strings.TrimSpace(doc.Project.RequiresPython); v != "" {
		return PythonRequirement{
			Source: "pyproject.toml#project.requires-python", Constraint: v, IsExact: false,
		}, true, nil
	}
	if v, ok := doc.Tool.Poetry.Dependencies["python"]; ok {
		switch s := v.(type) {
		case string:
			return PythonRequirement{
				Source: "pyproject.toml#tool.poetry.dependencies.python", Constraint: s, IsExact: false,
			}, true, nil
		}
	}
	return PythonRequirement{}, false, nil
}
