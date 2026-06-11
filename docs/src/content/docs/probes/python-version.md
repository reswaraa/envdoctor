---
title: python-version
description: Checks that the local Python interpreter satisfies the repo's declared version.
---

| Field | Value |
|---|---|
| Probe ID | `python-version` |
| Category | runtime |
| Severity | error |
| Inferred from | `pyproject.toml` (`project.requires-python`, `tool.poetry.dependencies.python`), `.python-version`, `.tool-versions`, `mise.toml` |

## What it means

The repo declares a Python version (or constraint) your local interpreter doesn't satisfy. Common pain: a `pyproject.toml` requires `>=3.11` but `python3` on PATH is 3.9; `pip install` fails or installs a stale wheel.

## How it's detected

1. The probe reads every supported manifest. `pyproject.toml`'s short forms (`^3.11`, `~=3.11`) expand into the canonical semver constraint Python actually uses.
2. The probe tries `python3 --version` first, then `python --version`.
3. The first one that returns is matched against the constraint.

## Common causes

- macOS ships an old `/usr/bin/python3`; the version manager isn't on PATH for this shell.
- `pyenv shell` hasn't been run in this terminal session.
- A virtualenv is active for an older interpreter than the project requires.

## Recipes

The probe selects one fix based on the tools available on your machine. See the [YAML source](https://github.com/reswaraa/envdoctor/blob/main/internal/recipes/library/python-version.yaml) for the full Fix definitions.

<!-- BEGIN auto-recipes -->

| Fix | Class | When | Fallback |
|---|---|---|---|
| `mise-install-python` | safe | has_tool=mise |  |
| `uv-install-python` | safe | has_tool=uv |  |
| `pyenv-install-python` | safe | has_tool=pyenv |  |
| `brew-install-python` | shared | has_tool=brew | yes |

<!-- END auto-recipes -->
