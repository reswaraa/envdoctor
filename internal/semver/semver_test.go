// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package semver

import (
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"18.17.0", "18.17.0", 0},
		{"18.17.0", "20.10.0", -1},
		{"20.10.0", "18.17.0", 1},
		{"v20.10.0", "20.10.0", 0},
		{"20.10.0", "v20.10.0", 0},
		{"20", "20.0.0", 0},
		{"20.10", "20.10.0", 0},
		{"  20.10.0  ", "20.10.0", 0},
	}
	for _, c := range cases {
		got, err := Compare(c.a, c.b)
		if err != nil {
			t.Errorf("Compare(%q, %q): %v", c.a, c.b, err)
			continue
		}
		if got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCompare_InvalidReturnsError(t *testing.T) {
	if _, err := Compare("not-a-version", "1.0.0"); err == nil {
		t.Error("expected error for invalid left side")
	}
	if _, err := Compare("1.0.0", "lts/iron"); err == nil {
		t.Error("expected error for invalid right side")
	}
}

func TestSatisfies(t *testing.T) {
	cases := []struct {
		version, constraint string
		want                bool
	}{
		// Caret: same major
		{"20.10.0", "^20.0.0", true},
		{"20.10.0", "^20", true},
		{"21.0.0", "^20.0.0", false},
		{"18.17.0", "^20.0.0", false},

		// Tilde: same major.minor
		{"20.10.5", "~20.10.0", true},
		{"20.11.0", "~20.10.0", false},

		// Exact and >=
		{"20.10.0", "20.10.0", true},
		{"20.10.0", "20.10.1", false},
		{"20.10.0", ">=18.0.0", true},
		{"20.10.0", ">=18", true},
		{"17.0.0", ">=18", false},

		// X-ranges
		{"20.10.0", "20.x", true},
		{"20.10.0", "20.x.x", true},
		{"21.0.0", "20.x", false},

		// Wildcard / star
		{"20.10.0", "*", true},
		{"0.0.1", "*", true},

		// Whitespace tolerance
		{"20.10.0", "  ^20.0.0  ", true},
	}
	for _, c := range cases {
		got, err := Satisfies(c.version, c.constraint)
		if err != nil {
			t.Errorf("Satisfies(%q, %q): %v", c.version, c.constraint, err)
			continue
		}
		if got != c.want {
			t.Errorf("Satisfies(%q, %q) = %v, want %v", c.version, c.constraint, got, c.want)
		}
	}
}

func TestSatisfies_InvalidReturnsError(t *testing.T) {
	if _, err := Satisfies("not-a-version", "^20"); err == nil {
		t.Error("expected error for invalid version")
	}
	if _, err := Satisfies("20.0.0", "not-a-constraint"); err == nil {
		t.Error("expected error for invalid constraint")
	}
}

func TestMajor(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"20.10.0", 20},
		{"v20.10.0", 20},
		{"3.11.2", 3},
		{"1.0.0", 1},
		{"18", 18},
		{"  20.10  ", 20},
	}
	for _, c := range cases {
		got, err := Major(c.in)
		if err != nil {
			t.Errorf("Major(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Major(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestMajor_InvalidReturnsError(t *testing.T) {
	_, err := Major("lts/iron")
	if err == nil {
		t.Fatal("expected error for non-numeric version")
	}
	if !strings.Contains(err.Error(), "lts/iron") {
		t.Errorf("error should mention the input; got: %v", err)
	}
}
