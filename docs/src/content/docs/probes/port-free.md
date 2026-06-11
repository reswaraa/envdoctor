---
title: port-free
description: Checks that TCP ports the repo's services want to bind are free on your machine.
---

| Field | Value |
|---|---|
| Probe ID | `port-free` |
| Category | ports |
| Severity | error |
| Inferred from | `docker-compose.yml` (`services.*.ports`) |

## What it means

The repo's `docker-compose.yml` declares ports its services will publish on the host, and one or more of those ports is already in use locally. `docker compose up` will fail at the first collision with a `bind: address already in use` message that doesn't say which service is holding the port.

## How it's detected

1. The probe walks `docker-compose.yml`'s `services.*.ports` entries. Both short form (`"5432:5432"`) and long form (`{ published: 5432, target: 5432 }`) are supported.
2. Port-range entries (`"3000-3010:3000-3010"`) are dropped — opportunistic port scanning would be expensive and noisy.
3. For each port, the probe attempts `net.Listen("tcp", "127.0.0.1:<port>")`. If the bind succeeds the port is free; the listener is closed immediately.
4. If the bind fails, the probe best-effort identifies the holder by running `lsof -ti :<port>` then `ps -p <PID> -o comm=` with a 1.5s budget. The holder name (e.g. `postgres`) is surfaced for the recipe template.

The probe emits one finding per colliding port — not one aggregate — so the repair order is per-port.

## Common causes

- Postgres / Redis / RabbitMQ installed via brew and running as a launchd service.
- A previous `docker compose up` that didn't shut down cleanly.
- A globally-installed dev server already running on the default port.

## Recipes

The destructive fix (`kill $(lsof -ti :<PORT>)`) always prompts. The `brew services list` fallback applies when the holder looks like a brew service. See the [YAML source](https://github.com/reswaraa/envdoctor/blob/main/internal/recipes/library/port-free.yaml).

<!-- BEGIN auto-recipes -->

| Fix | Class | When | Fallback |
|---|---|---|---|
| `kill-port-holder` | destructive | * |  |
| `brew-services-list` | shared | has_tool=brew | yes |

<!-- END auto-recipes -->
