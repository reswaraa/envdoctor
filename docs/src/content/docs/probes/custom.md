---
title: custom (.envdoctor.yaml checks)
description: Meta-probe that evaluates declarative checks declared in the repo's .envdoctor.yaml.
---

| Field | Value |
|---|---|
| Probe ID | `custom` |
| Category | custom |
| Severity | error |
| Inferred from | `.envdoctor.yaml#checks` |

## What it means

`custom` isn't a "real" probe — it's the meta-probe that runs the declarative checks a maintainer added to `.envdoctor.yaml` to extend envdoctor's inference. Each finding's evidence names which check produced it.

The four supported check types:

| Type | Asserts |
|---|---|
| `tool_version` | A named tool is on PATH at a version satisfying a semver constraint. |
| `port_free` | A TCP port is free to bind. |
| `env_required` | One or more env var names are set in the process env or `.env`. |
| `command_present` | A command is on PATH. |

See [Config (.envdoctor.yaml)](https://reswaraa.github.io/envdoctor/recipes/schema/) for the full reference.

## How it's detected

1. `.envdoctor.yaml` is loaded by `internal/config` (returns nil if the file is absent — config is optional).
2. Each `checks` entry is dispatched to its handler by the discriminator `type:` field.
3. Each failing check produces one finding with `Probe = custom` and evidence pointing at the originating check.

The custom probe has no automated recipe — the finding's evidence explains what's wrong and the maintainer's documentation should explain how to fix it. Future probes may attach recipes for the curated check types.

## Common causes

- A check was added to `.envdoctor.yaml` for a tool/version the contributor doesn't have.
- A `port_free` check trips because the contributor is running a different stack locally.
- An `env_required` check covers a new env var that was never documented in `.env.example`.

## Recipes

The custom probe has no automated recipe today. The [Config schema page](https://reswaraa.github.io/envdoctor/recipes/schema/) is the primary remediation reference; repository maintainers should document expected setup in their own README.

<!-- BEGIN auto-recipes -->

_No recipes today — open an issue with a debug bundle so a recipe can be authored._

<!-- END auto-recipes -->
