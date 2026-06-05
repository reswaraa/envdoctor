// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"reflect"
	"testing"
)

func TestParseShortPortString(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"5432", 5432},
		{"5432:5432", 5432},
		{"127.0.0.1:5432:5432", 5432},
		{"5432:5432/tcp", 5432},
		{"127.0.0.1:5432:5432/udp", 5432},
		{"8000-8005:5000-5005", 0}, // ranges not supported
		{"", 0},
		{"abc", 0},
		{"99999", 0},
		{"0", 0},
	}
	for _, c := range cases {
		if got := parseShortPortString(c.in); got != c.want {
			t.Errorf("parseShortPortString(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestInferPorts_EmptyRepoReturnsNothing(t *testing.T) {
	reqs, err := InferPorts(t.TempDir())
	if err != nil {
		t.Fatalf("InferPorts: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("expected 0; got %+v", reqs)
	}
}

func TestInferPorts_ShortForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", `services:
  db:
    image: postgres
    ports:
      - "5432:5432"
      - "127.0.0.1:6379:6379"
  web:
    image: nginx
    ports:
      - "80"
`)
	reqs, err := InferPorts(dir)
	if err != nil {
		t.Fatalf("InferPorts: %v", err)
	}
	want := []PortRequirement{
		{Source: "docker-compose.yml#services.web.ports", Port: 80},
		{Source: "docker-compose.yml#services.db.ports", Port: 5432},
		{Source: "docker-compose.yml#services.db.ports", Port: 6379},
	}
	if !reflect.DeepEqual(reqs, want) {
		t.Errorf("got %+v, want %+v", reqs, want)
	}
}

func TestInferPorts_LongForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", `services:
  db:
    image: postgres
    ports:
      - target: 5432
        published: 5432
        protocol: tcp
      - target: 9090
        published: 9090
`)
	reqs, err := InferPorts(dir)
	if err != nil {
		t.Fatalf("InferPorts: %v", err)
	}
	want := []PortRequirement{
		{Source: "docker-compose.yml#services.db.ports", Port: 5432},
		{Source: "docker-compose.yml#services.db.ports", Port: 9090},
	}
	if !reflect.DeepEqual(reqs, want) {
		t.Errorf("got %+v, want %+v", reqs, want)
	}
}

func TestInferPorts_DeduplicatesAcrossServices(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", `services:
  a:
    ports: ["5432:5432"]
  b:
    ports: ["5432:5432"]
`)
	reqs, err := InferPorts(dir)
	if err != nil {
		t.Fatalf("InferPorts: %v", err)
	}
	if len(reqs) != 1 || reqs[0].Port != 5432 {
		t.Errorf("expected one dedup'd 5432; got %+v", reqs)
	}
}

func TestInferPorts_SkipsRangesAndMalformed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", `services:
  app:
    ports:
      - "8000-8005:5000-5005"
      - "not-a-port"
      - "9090:9090"
`)
	reqs, err := InferPorts(dir)
	if err != nil {
		t.Fatalf("InferPorts: %v", err)
	}
	if len(reqs) != 1 || reqs[0].Port != 9090 {
		t.Errorf("expected only the well-formed 9090; got %+v", reqs)
	}
}
