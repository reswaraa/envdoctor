---
title: docker-running
description: Distinguishes a missing docker CLI from a stopped daemon.
---

| Field | Value |
|---|---|
| Probe ID | `docker-running` |
| Category | docker |
| Severity | error |
| Inferred from | `Dockerfile` family, `docker-compose.yml` family (any of: `docker-compose.yml`, `docker-compose.yaml`, `compose.yml`, `compose.yaml`) |

## What it means

The repo expects Docker (or has a Dockerfile / compose file) and either the `docker` CLI isn't installed or the daemon isn't running. The two failure modes look identical to `docker compose up` ("Cannot connect to the Docker daemon") but their fixes are completely different — one is install, the other is start. This probe distinguishes them so the recipe is correct.

## How it's detected

1. The probe applies only if the repo contains a Dockerfile or compose file (`HasDockerSignals`).
2. `Facts.HasTool("docker")` — if false, the CLI is missing. Emits the **cli-missing** finding.
3. Otherwise, `docker info` is exec'd with a 3-second timeout. If it errors or times out, the daemon isn't running. Emits the **daemon-down** finding.

The 3-second cap matters: a hung daemon connection can take minutes to fail by default; envdoctor's 2-second budget would blow if we let it spin.

## Common causes

### cli-missing

- Fresh dev machine — never installed Docker Desktop / Colima.
- Linux distro where `docker` was uninstalled in favor of `podman`.

### daemon-down

- Docker Desktop closed; restart it from Applications.
- Colima not started — `colima start` after a reboot.
- Linux: `systemctl is-active docker` returns inactive.
- WSL2 with Docker Desktop not configured to integrate with the running distro.

## Recipes

Two separate recipes — `docker-cli-missing` for the install path, `docker-daemon-down` for the start-the-daemon path. The probe attaches whichever matches the finding it emitted. See the YAML source for [docker-cli-missing](https://github.com/reswaraa/envdoctor/blob/main/internal/recipes/library/docker-cli-missing.yaml) and [docker-daemon-down](https://github.com/reswaraa/envdoctor/blob/main/internal/recipes/library/docker-daemon-down.yaml).

<!-- BEGIN auto-recipes -->

### `docker-cli-missing`

| Fix | Class | When | Fallback |
|---|---|---|---|
| `brew-cask-docker` | shared | os=darwin, has_tool=brew |  |
| `apt-install-docker` | privileged | os=linux, has_tool=apt | yes |

### `docker-daemon-down`

| Fix | Class | When | Fallback |
|---|---|---|---|
| `colima-start` | safe | has_tool=colima |  |
| `open-docker-desktop` | safe | os=darwin |  |
| `systemctl-start-docker` | privileged | os=linux | yes |

<!-- END auto-recipes -->
