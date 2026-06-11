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

Two recipes, picked by the probe based on which finding was emitted:

- [`docker-cli-missing`](https://github.com/reswaraa/envdoctor/blob/main/internal/recipes/library/docker-cli-missing.yaml) — `brew install --cask docker` (shared) or `apt-get install docker.io` (privileged).
- [`docker-daemon-down`](https://github.com/reswaraa/envdoctor/blob/main/internal/recipes/library/docker-daemon-down.yaml) — `colima start`, `open -a Docker`, or `systemctl start docker` depending on the platform.
