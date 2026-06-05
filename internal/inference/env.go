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
	"sort"
	"strings"
)

// EnvRequirement is one environment variable the repo expects to be set.
// Source is the manifest file that surfaced the requirement.
type EnvRequirement struct {
	Source string
	Key    string
}

// envKeyRE matches a valid POSIX-shell-ish env variable name.
var envKeyRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// composeVarRE matches `${...}` style interpolations in compose files.
// The inner content may include `:-default`, `:?error`, `:=default` etc.;
// we filter those out at the call site as "optional".
var composeVarRE = regexp.MustCompile(`\$\{([^}]+)\}`)

// InferEnv collects environment variables the repo requires, in
// stable order across sources:
//
//	.env.example
//	docker-compose.yml / docker-compose.yaml / compose.yml / compose.yaml
//
// Duplicate keys across sources are de-duplicated, keeping the first
// Source that surfaced the key.
func InferEnv(root string) ([]EnvRequirement, error) {
	var out []EnvRequirement
	seen := map[string]bool{}

	appendUnique := func(reqs []EnvRequirement) {
		for _, r := range reqs {
			if seen[r.Key] {
				continue
			}
			seen[r.Key] = true
			out = append(out, r)
		}
	}

	reqs, err := readEnvExample(root)
	if err != nil {
		return nil, err
	}
	appendUnique(reqs)

	reqs, err = readComposeFiles(root)
	if err != nil {
		return nil, err
	}
	appendUnique(reqs)

	return out, nil
}

func readEnvExample(root string) ([]EnvRequirement, error) {
	keys, err := ReadEnvKeys(filepath.Join(root, ".env.example"))
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	out := make([]EnvRequirement, 0, len(keys))
	for _, k := range keys {
		out = append(out, EnvRequirement{Source: ".env.example", Key: k})
	}
	return out, nil
}

// ReadEnvKeys parses a .env-style file and returns the ordered list of
// keys present. Returns nil if the file does not exist. Lines starting
// with `#` and blank lines are skipped. Values are ignored — only key
// names are returned (callers writing redacted bundles depend on this).
func ReadEnvKeys(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(strings.TrimPrefix(line[:eq], "export "))
		if !envKeyRE.MatchString(key) {
			continue
		}
		out = append(out, key)
	}
	return out, nil
}

func readComposeFiles(root string) ([]EnvRequirement, error) {
	names := []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}
	var out []EnvRequirement
	seen := map[string]bool{}
	for _, name := range names {
		p := filepath.Join(root, name)
		b, err := os.ReadFile(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		keys := extractComposeVarRefs(string(b))
		for _, k := range keys {
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, EnvRequirement{Source: name, Key: k})
		}
	}
	// extractComposeVarRefs preserves first-seen order across the file;
	// sort within a file is *not* desired because user-meaningful order
	// matters for nothing here, but we sort the cross-file dedup output
	// for determinism so the test output is stable.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Source == out[j].Source {
			return out[i].Key < out[j].Key
		}
		return false
	})
	return out, nil
}

// extractComposeVarRefs scans a docker-compose file for `${VAR}` style
// references and returns those that the user must provide (i.e. with no
// `:-default`, `:?error`, `:=default`, or `:+value` form, which would
// mean compose has its own fallback).
func extractComposeVarRefs(content string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range composeVarRE.FindAllStringSubmatch(content, -1) {
		inner := strings.TrimSpace(m[1])
		// Optional forms have a ':' modifier — compose provides a default.
		if strings.ContainsAny(inner, ":?-=+") {
			continue
		}
		if !envKeyRE.MatchString(inner) {
			continue
		}
		if seen[inner] {
			continue
		}
		seen[inner] = true
		out = append(out, inner)
	}
	return out
}
