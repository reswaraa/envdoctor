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

	"gopkg.in/yaml.v3"
)

// CommandRequirement is one binary name a repo script invokes.
// Source is the manifest file (and a sub-path where relevant) that
// surfaced the requirement.
type CommandRequirement struct {
	Source  string
	Command string
}

// binaryNameRE accepts only well-formed binary names. Rejects shell
// vars ($GO), relative paths (./manage.py), pipes, and other things
// that obviously aren't system binaries.
var binaryNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

// skipBuiltins are commands the path probe must NOT flag as missing:
// shell builtins, POSIX standard utilities, language runtimes already
// covered by other probes, common dev tools assumed everywhere.
var skipBuiltins = map[string]bool{
	// shell builtins / control
	"cd": true, "echo": true, "exit": true, "export": true, "pwd": true,
	"set": true, "source": true, "test": true, "true": true, "false": true,
	// POSIX coreutils
	"awk": true, "basename": true, "cat": true, "chmod": true, "cp": true,
	"date": true, "dirname": true, "env": true, "find": true, "grep": true,
	"head": true, "ls": true, "mkdir": true, "mv": true, "printf": true,
	"rm": true, "rmdir": true, "sed": true, "sleep": true, "tail": true,
	"tar": true, "touch": true, "tr": true, "uname": true, "which": true,
	// language runtimes covered by other probes
	"node": true, "npm": true, "npx": true, "pnpm": true, "yarn": true,
	"python": true, "python3": true, "pip": true, "pip3": true,
	"ruby": true, "gem": true, "bundle": true, "bundler": true,
	"go": true, "gofmt": true, "rustc": true, "cargo": true,
	// shells
	"bash": true, "sh": true, "zsh": true, "fish": true,
	// covered by their own probes / nearly universal
	"git": true, "docker": true,
}

// InferCommands extracts binary names referenced by repo scripts in
// Makefile, Procfile, and docker-compose.yml#services.*.command/
// entrypoint. Returns deduplicated requirements; first source wins.
//
// package.json#scripts is intentionally NOT scanned: scripts run with
// node_modules/.bin on PATH, so locally-installed tools (tsc, vite,
// next) would generate false positives the user can't act on. The
// generic "your repo needs a global tool" use case is best served by
// Makefile / Procfile / compose entries.
func InferCommands(root string) ([]CommandRequirement, error) {
	var out []CommandRequirement
	seen := map[string]bool{}

	appendUnique := func(reqs []CommandRequirement) {
		for _, r := range reqs {
			if seen[r.Command] {
				continue
			}
			if skipBuiltins[r.Command] {
				continue
			}
			seen[r.Command] = true
			out = append(out, r)
		}
	}

	for _, reader := range []func(string) ([]CommandRequirement, error){
		readMakefileCommands,
		readProcfileCommands,
		readComposeCommands,
	} {
		reqs, err := reader(root)
		if err != nil {
			return nil, err
		}
		appendUnique(reqs)
	}
	return out, nil
}

func readMakefileCommands(root string) ([]CommandRequirement, error) {
	var path, name string
	for _, n := range []string{"Makefile", "GNUmakefile", "makefile"} {
		p := filepath.Join(root, n)
		if _, err := os.Stat(p); err == nil {
			path, name = p, n
			break
		}
	}
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}

	var out []CommandRequirement
	seen := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		// Recipe lines start with a tab.
		if !strings.HasPrefix(line, "\t") {
			continue
		}
		// Drop the leading tab and Make's silent / ignore / always-exec
		// recipe modifiers.
		line = strings.TrimLeft(line, "\t")
		line = strings.TrimLeft(line, "@-+")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cmd := firstWord(line)
		if !validBinaryName(cmd) {
			continue
		}
		if seen[cmd] {
			continue
		}
		seen[cmd] = true
		out = append(out, CommandRequirement{Source: name, Command: cmd})
	}
	return out, nil
}

func readProcfileCommands(root string) ([]CommandRequirement, error) {
	b, err := os.ReadFile(filepath.Join(root, "Procfile"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read Procfile: %w", err)
	}
	var out []CommandRequirement
	seen := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		cmd := firstWord(line[i+1:])
		if !validBinaryName(cmd) {
			continue
		}
		if seen[cmd] {
			continue
		}
		seen[cmd] = true
		out = append(out, CommandRequirement{Source: "Procfile", Command: cmd})
	}
	return out, nil
}

func readComposeCommands(root string) ([]CommandRequirement, error) {
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		p := filepath.Join(root, name)
		b, err := os.ReadFile(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		return parseComposeCommands(name, b)
	}
	return nil, nil
}

func parseComposeCommands(file string, raw []byte) ([]CommandRequirement, error) {
	var doc struct {
		Services map[string]struct {
			Command    any `yaml:"command"`
			Entrypoint any `yaml:"entrypoint"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}
	serviceNames := make([]string, 0, len(doc.Services))
	for n := range doc.Services {
		serviceNames = append(serviceNames, n)
	}
	sort.Strings(serviceNames)

	var out []CommandRequirement
	seen := map[string]bool{}
	for _, svc := range serviceNames {
		entries := []struct {
			kind string
			val  any
		}{
			{"command", doc.Services[svc].Command},
			{"entrypoint", doc.Services[svc].Entrypoint},
		}
		for _, e := range entries {
			cmd := firstTokenOfComposeAny(e.val)
			if !validBinaryName(cmd) {
				continue
			}
			if seen[cmd] {
				continue
			}
			seen[cmd] = true
			out = append(out, CommandRequirement{
				Source:  fmt.Sprintf("%s#services.%s.%s", file, svc, e.kind),
				Command: cmd,
			})
		}
	}
	return out, nil
}

func firstTokenOfComposeAny(v any) string {
	switch x := v.(type) {
	case string:
		return firstWord(x)
	case []any:
		if len(x) == 0 {
			return ""
		}
		if s, ok := x[0].(string); ok {
			return firstWord(s)
		}
	}
	return ""
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	for i, r := range s {
		if r == ' ' || r == '\t' {
			return s[:i]
		}
	}
	return s
}

func validBinaryName(s string) bool {
	return binaryNameRE.MatchString(s)
}
