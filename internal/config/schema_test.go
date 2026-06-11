// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestPublishedJSONSchemaIsParseable pins three things about the
// JSON Schema we ship at docs/public/schemas/v1/config.json
// (served by Astro at the URL https://envdoctor.dev/schemas/v1/config.json):
//
//  1. It parses as valid JSON.
//  2. The $id is the forever-stable URL contract.
//  3. The enum of check `type:` values matches the Go-side constants —
//     adding a new CheckXxx constant requires updating the schema in
//     the same PR, or this test fails loudly.
func TestPublishedJSONSchemaIsParseable(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "docs", "public", "schemas", "v1", "config.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parse schema: %v", err)
	}

	if id, _ := doc["$id"].(string); id != "https://envdoctor.dev/schemas/v1/config.json" {
		t.Errorf("$id changed (forever-stable URL contract); got %q", id)
	}

	defs, _ := doc["$defs"].(map[string]any)
	check, _ := defs["Check"].(map[string]any)
	props, _ := check["properties"].(map[string]any)
	typeProp, _ := props["type"].(map[string]any)
	rawEnum, _ := typeProp["enum"].([]any)

	schemaTypes := map[string]bool{}
	for _, v := range rawEnum {
		if s, ok := v.(string); ok {
			schemaTypes[s] = true
		}
	}

	goTypes := []string{
		CheckToolVersion,
		CheckPortFree,
		CheckEnvRequired,
		CheckCommandPresent,
	}
	for _, gt := range goTypes {
		if !schemaTypes[gt] {
			t.Errorf("Go-side check type %q missing from JSON Schema enum %v", gt, schemaTypes)
		}
	}
	if len(schemaTypes) != len(goTypes) {
		t.Errorf("schema enum (%v) and Go constants (%v) have different sizes — keep them in lockstep",
			schemaTypes, goTypes)
	}
}

// repoRoot walks up from the test cwd until it finds go.mod. Lets the
// test resolve docs/ without hardcoding ../.. parent counts.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not find go.mod above %s", dir)
	return ""
}
