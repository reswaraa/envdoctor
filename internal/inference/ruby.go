// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// RubyRequirement is one Ruby runtime version constraint.
type RubyRequirement struct {
	Source     string
	Constraint string
	IsExact    bool
}

// AsConstraint expands short version forms the way Ruby version
// managers (rbenv, chruby, asdf) interpret them.
//
//	"3.2"      -> "~3.2.0"
//	"3.2.2"    -> "3.2.2"
//	"~> 3.2"   -> "^3.2"  (RubyGems "~>" major-minor maps to caret)
//	"~> 3.2.0" -> "~3.2.0"
func (r RubyRequirement) AsConstraint() string {
	raw := strings.TrimSpace(r.Constraint)
	if strings.HasPrefix(raw, "~>") {
		v := strings.TrimSpace(strings.TrimPrefix(raw, "~>"))
		parts := strings.Split(v, ".")
		switch len(parts) {
		case 1:
			return "^" + v + ".0.0"
		case 2:
			return "^" + v + ".0"
		default:
			return "~" + v
		}
	}
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

// gemfileRubyRE matches `ruby '<version>'` or `ruby "<version>"`
// (with optional `=>`-style key prefix and trailing whitespace).
var gemfileRubyRE = regexp.MustCompile(`(?m)^\s*ruby\s+['"]([^'"]+)['"]`)

// InferRuby collects Ruby version requirements from:
//
//	.ruby-version, .tool-versions, .mise.toml / mise.toml,
//	Gemfile (the `ruby '...'` directive)
func InferRuby(root string) ([]RubyRequirement, error) {
	var out []RubyRequirement
	for _, fn := range []func(string) (RubyRequirement, bool, error){
		readRubyVersionFile,
		readToolVersionsRuby,
		readMiseTOMLRuby,
		readGemfileRuby,
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

func readRubyVersionFile(root string) (RubyRequirement, bool, error) {
	r, ok, err := readSingleLineFile(root, ".ruby-version")
	if err != nil || !ok {
		return RubyRequirement{}, ok, err
	}
	return RubyRequirement(r), true, nil
}

func readToolVersionsRuby(root string) (RubyRequirement, bool, error) {
	p := filepath.Join(root, ".tool-versions")
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return RubyRequirement{}, false, nil
		}
		return RubyRequirement{}, false, fmt.Errorf("read .tool-versions: %w", err)
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
		if fields[0] == "ruby" {
			return RubyRequirement{Source: ".tool-versions", Constraint: fields[1], IsExact: true}, true, nil
		}
	}
	return RubyRequirement{}, false, nil
}

func readMiseTOMLRuby(root string) (RubyRequirement, bool, error) {
	for _, name := range []string{".mise.toml", "mise.toml"} {
		p := filepath.Join(root, name)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		var doc struct {
			Tools map[string]any `toml:"tools"`
		}
		if _, err := toml.DecodeFile(p, &doc); err != nil {
			return RubyRequirement{}, false, fmt.Errorf("parse %s: %w", name, err)
		}
		v, ok := doc.Tools["ruby"]
		if !ok {
			continue
		}
		switch s := v.(type) {
		case string:
			return RubyRequirement{Source: name, Constraint: s, IsExact: true}, true, nil
		case map[string]any:
			if str, ok := s["version"].(string); ok {
				return RubyRequirement{Source: name, Constraint: str, IsExact: true}, true, nil
			}
		}
	}
	return RubyRequirement{}, false, nil
}

func readGemfileRuby(root string) (RubyRequirement, bool, error) {
	b, err := os.ReadFile(filepath.Join(root, "Gemfile"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return RubyRequirement{}, false, nil
		}
		return RubyRequirement{}, false, fmt.Errorf("read Gemfile: %w", err)
	}
	m := gemfileRubyRE.FindStringSubmatch(string(b))
	if m == nil {
		return RubyRequirement{}, false, nil
	}
	v := strings.TrimSpace(m[1])
	return RubyRequirement{Source: "Gemfile#ruby", Constraint: v, IsExact: false}, true, nil
}
