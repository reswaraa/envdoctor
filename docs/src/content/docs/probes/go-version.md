---
title: go-version
description: Checks that the local Go toolchain satisfies the repo's declared version.
---

| Field | Value |
|---|---|
| Probe ID | `go-version` |
| Category | runtime |
| Severity | error |
| Inferred from | `go.mod` (`go` directive), `.tool-versions`, `mise.toml` |

## What it means

The repo's `go.mod` declares `go X.Y` (or stricter) and your local `go version` is lower. Building will produce inscrutable errors about syntax or stdlib symbols introduced in a later release.

## How it's detected

1. Parse the `go` directive from `go.mod`. Translate `go 1.22` into a `>=1.22` constraint (Go's "newer is fine" semantics).
2. `go version` is exec'd, the `go1.X.Y` token is extracted.
3. Mismatch produces one finding.

If `go.mod` is absent the probe is silent.

## Common causes

- Apple Silicon `/usr/local/go` from a year-old binary tarball.
- `mise` or `asdf` configured to pin Go for the repo but not yet `mise install`-ed.
- Linux distro packages (`apt install golang`) lagging months behind upstream.

## Recipes

The probe selects one fix based on the tools available on your machine. See the [YAML source](https://github.com/reswaraa/envdoctor/blob/main/internal/recipes/library/go-version.yaml) for the full Fix definitions.

<!-- BEGIN auto-recipes -->

| Fix | Class | When | Fallback |
|---|---|---|---|
| `mise-install-go` | safe | has_tool=mise |  |
| `asdf-install-go` | safe | has_tool=asdf |  |
| `brew-install-go` | shared | has_tool=brew | yes |

<!-- END auto-recipes -->
