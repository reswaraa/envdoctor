// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// PortRequirement is one TCP host port the repo expects to be free
// locally. Source identifies the compose file plus service name.
type PortRequirement struct {
	Source string
	Port   int
}

// InferPorts collects host ports from any docker-compose family file
// under root. Returns ports sorted ascending with duplicates removed.
func InferPorts(root string) ([]PortRequirement, error) {
	names := []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}
	var all []PortRequirement
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		reqs, err := parseComposePorts(name, b)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		all = append(all, reqs...)
	}
	return dedupAndSortPorts(all), nil
}

func parseComposePorts(file string, raw []byte) ([]PortRequirement, error) {
	var doc struct {
		Services map[string]struct {
			Ports []composePort `yaml:"ports"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	serviceNames := make([]string, 0, len(doc.Services))
	for name := range doc.Services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)

	var out []PortRequirement
	for _, svc := range serviceNames {
		for _, p := range doc.Services[svc].Ports {
			port := p.HostPort()
			if port == 0 {
				continue
			}
			out = append(out, PortRequirement{
				Source: fmt.Sprintf("%s#services.%s.ports", file, svc),
				Port:   port,
			})
		}
	}
	return out, nil
}

// composePort handles both the short string form ("5432:5432",
// "5432", "127.0.0.1:5432:5432", "5432:5432/tcp") and the long
// mapping form ({target: 5432, published: 8000}).
type composePort struct {
	raw       string
	published int
}

func (p *composePort) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		p.raw = node.Value
		return nil
	case yaml.MappingNode:
		var m map[string]any
		if err := node.Decode(&m); err != nil {
			return err
		}
		if v, ok := m["published"]; ok {
			p.published = toIntSafe(v)
		}
		return nil
	default:
		return fmt.Errorf("ports: unsupported YAML node kind %v", node.Kind)
	}
}

// HostPort returns the host-side TCP port number, or 0 if the entry
// cannot be parsed (port ranges, malformed strings, missing fields).
func (p composePort) HostPort() int {
	if p.published > 0 {
		return p.published
	}
	if p.raw == "" {
		return 0
	}
	return parseShortPortString(p.raw)
}

func parseShortPortString(s string) int {
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	var hostStr string
	switch len(parts) {
	case 1:
		hostStr = parts[0]
	case 2:
		hostStr = parts[0]
	case 3:
		hostStr = parts[1]
	default:
		return 0
	}
	if strings.Contains(hostStr, "-") {
		return 0 // port range, not supported in MVP
	}
	n, err := strconv.Atoi(hostStr)
	if err != nil || n <= 0 || n > 65535 {
		return 0
	}
	return n
}

func toIntSafe(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, err := strconv.Atoi(x)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

func dedupAndSortPorts(in []PortRequirement) []PortRequirement {
	seen := map[int]bool{}
	out := make([]PortRequirement, 0, len(in))
	for _, r := range in {
		if seen[r.Port] {
			continue
		}
		seen[r.Port] = true
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out
}
