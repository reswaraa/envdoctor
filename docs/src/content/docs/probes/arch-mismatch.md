---
title: arch-mismatch
description: Catches known-x86-only native deps locked into projects running on Apple Silicon.
---

| Field | Value |
|---|---|
| Probe ID | `arch-mismatch` |
| Category | architecture |
| Severity | error |
| Inferred from | `package-lock.json` (v1, v2, v3) |
| Applies to | `arm64` only |

## What it means

Your machine is arm64 (Apple Silicon, AWS Graviton, …) and the repo's lockfile pins versions of native-dep packages that pre-date arm64 wheels. `npm install` will appear to succeed via Rosetta emulation, then break at runtime with a `wrong ELF class` or `Image not found` error.

## How it's detected

The probe walks `package-lock.json` (all three schemas) for known-x86-only dep versions:

| Package | Last x86-only version |
|---|---|
| `sharp` | `< 0.33` |
| `canvas` | `< 2.11` |
| `cypress` | `< 13` |

A match produces one finding per offending dep with the minimum arm-compatible version surfaced in the summary.

Currently `yarn.lock` and `pnpm-lock.yaml` are out of scope — adding them is a probe extension. Docker-image-arch detection (`platform: linux/amd64` in compose) is also deferred.

## Common causes

- Lockfile committed when the repo was primarily x86; arm contributor joined later.
- A maintainer pinned an old `sharp` to dodge a bug fix that broke their pipeline.
- Stale `package-lock.json` that hasn't been regenerated since the bump.

## Recipes

See the [recipe library](https://github.com/reswaraa/envdoctor/blob/main/internal/recipes/library/arch-mismatch.yaml). The single fix is `npm install <pkg>@<fixed-version>` which both bumps the lockfile and rebuilds against the arm64 prebuild.
