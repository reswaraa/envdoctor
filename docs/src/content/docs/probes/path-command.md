---
title: path-command
description: Checks that commands referenced in build files are available on your PATH.
---

| Field | Value |
|---|---|
| Probe ID | `path-command` |
| Category | path |
| Severity | error |
| Inferred from | `Makefile` / `GNUmakefile`, `Procfile`, `docker-compose.yml` (`services.*.command` / `entrypoint`) |

## What it means

The repo's build / orchestration files invoke commands (e.g. `psql`, `kubectl`, `ffmpeg`, `redis-cli`) that aren't on your PATH. Running the affected target produces a `command not found` and the test or compose step skips silently — worse than an explicit error.

## How it's detected

1. Each supported manifest is parsed for shell-callable tokens at recipe-call positions.
2. `package.json#scripts` is intentionally **not** scanned: an npm script can reference any binary under `node_modules/.bin` and false-positives there would be louder than the real signal.
3. Every distinct missing command produces one finding. Commands envdoctor maintains an install recipe for (the `commandPackages` map) get a recipe attached; others surface as exit code 2 ("envdoctor needs a recipe for X").

`justfile` is currently out of scope; a future probe may add it.

## Common causes

- A db client (`psql`, `mysql`) needed by a migration script.
- `kubectl`, `helm`, `terraform` referenced in a target but not installed.
- A specific build of `ffmpeg` or `imagemagick` from brew/apt.

## Recipes

The curated install list maps each command to `brew install <pkg>` (shared) or `apt-get install <pkg>` (privileged). For uncurated commands the probe emits a no-recipe finding (exit code 2). See the [YAML source](https://github.com/reswaraa/envdoctor/blob/main/internal/recipes/library/path-command-missing.yaml).

<!-- BEGIN auto-recipes -->

| Fix | Class | When | Fallback |
|---|---|---|---|
| `brew-install` | shared | has_tool=brew |  |
| `apt-install` | privileged | has_tool=apt | yes |

<!-- END auto-recipes -->
