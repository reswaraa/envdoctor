---
title: env-required
description: Checks that required environment variables are present in your shell or .env file.
---

| Field | Value |
|---|---|
| Probe ID | `env-required` |
| Category | environment |
| Severity | error |
| Inferred from | `.env.example`, `docker-compose.yml` (`${VAR}` references) |

## What it means

The repo declares env vars it expects at runtime (via a `.env.example` template or `${VAR}` interpolations in `docker-compose.yml`) and some of them are missing from both your shell and your local `.env` file. Without them the app fails on startup, usually with an opaque "undefined" error.

## How it's detected

1. `.env.example` lines matching `KEY=value` contribute key names. Values are **never read or stored** — only the key names enter the finding.
2. `docker-compose.yml` is parsed; `${VAR}` references in the parsed structure contribute key names. Default-value forms like `${VAR:-default}`, `${VAR:?error}`, `${VAR:=fallback}`, `${VAR:+set}` are **dropped** (a default means it's not required).
3. The probe checks both the process env and the parsed `.env` file at the repo root.
4. Keys missing from both produce one finding listing the missing names.

The structural guarantee: env values, file bodies, and shell secrets never enter the Finding. The redaction step in the debug bundle is belt-and-suspenders for paths; this probe is the source-of-truth that values stay out of the report.

## Common causes

- Fresh clone — `.env` doesn't exist yet (the recipe is just `cp -n .env.example .env`).
- A new env var was added to `.env.example` and you didn't pull-rebase.
- You exported the var in one shell but are running envdoctor from another.

## Recipes

The default fix is `cp -n .env.example .env` — idempotent (`-n` won't clobber an existing file). See the [YAML source](https://github.com/reswaraa/envdoctor/blob/main/internal/recipes/library/env-required.yaml).

<!-- BEGIN auto-recipes -->

| Fix | Class | When | Fallback |
|---|---|---|---|
| `copy-env-example` | safe | * |  |

<!-- END auto-recipes -->
