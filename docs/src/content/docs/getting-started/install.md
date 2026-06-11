---
title: Install
description: Get envdoctor on your PATH in under a minute.
---

## Global install (one machine, all your repos)

```sh
curl -fsSL https://envdoctor.dev/install.sh | sh
```

The installer:

1. Detects your OS (darwin / linux) and arch (amd64 / arm64).
2. Resolves the latest release tag.
3. Downloads the matching tarball + `sha256sums.txt`.
4. Verifies the SHA-256 against the published digest.
5. Installs to `~/.local/bin` (or `/usr/local/bin` if writable). Never auto-sudo.

Pin a specific version with `ENVDOCTOR_VERSION=v0.1.0`.

## Per-repo bootstrap (works for every contributor)

If you maintain a repo, run `envdoctor init` once. It writes:

- `./envdoctor` — a 45-line POSIX-sh bootstrap that pins envdoctor by SHA-256 and caches the binary at `~/.cache/envdoctor/versions/`.
- `.envdoctor.yaml` — a minimal config (schema version + min envdoctor version).

Commit both. Contributors then just run `./envdoctor scan` — no global install required.

## Upgrade

```sh
envdoctor upgrade
```

In-place atomic replacement, verifies SHA-256 before swap. Refuses to touch bootstrap-managed copies (re-run `envdoctor init --force` in the owning repo to bump that pin).
