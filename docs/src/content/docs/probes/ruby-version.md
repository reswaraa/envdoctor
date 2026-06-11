---
title: ruby-version
description: Checks that the local Ruby satisfies the repo's declared version.
---

| Field | Value |
|---|---|
| Probe ID | `ruby-version` |
| Category | runtime |
| Severity | error |
| Inferred from | `Gemfile` (`ruby 'X.Y.Z'`), `.ruby-version`, `.tool-versions`, `mise.toml` |

## What it means

The repo declares a Ruby version your local interpreter doesn't satisfy. `bundle install` will either refuse or silently produce a `Gemfile.lock` with the wrong BUNDLED WITH metadata.

## How it's detected

1. Read every supported manifest. `Gemfile` is regex-scanned for `ruby '...'` declarations.
2. `ruby --version` is exec'd; output like `ruby 3.2.2 (...)` is parsed to the version token.
3. Any mismatch produces one finding.

## Common causes

- System Ruby is 2.6 (macOS pre-Sonoma) and the repo wants 3.x.
- `rbenv shell` not run; the shim is on PATH but the active version disagrees.
- `chruby` setup that requires explicit sourcing per shell.

## Recipes

See the [recipe library](https://github.com/reswaraa/envdoctor/blob/main/internal/recipes/library/ruby-version.yaml). Tools tried in order: `mise`, `rbenv`, `asdf`, `brew`.
