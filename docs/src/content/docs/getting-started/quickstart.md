---
title: Quick start
description: From clone to a working dev loop in three commands.
---

## Three commands

```sh
git clone <some-repo>
cd <some-repo>
envdoctor scan
```

You get one of three outcomes:

- **Exit 0** — your machine is set up to run this repo.
- **Exit 1** — there are findings, and every one has a recipe envdoctor knows. Run `envdoctor fix` to walk through them.
- **Exit 2** — envdoctor saw something it doesn't have a recipe for yet. Open an issue with `envdoctor scan --bundle ./bundle.json` attached.

## Interactive repair

```sh
envdoctor fix
```

Walks each finding in repair order (runtime versions → port collisions → docker state → other). Single-key consent per recipe:

- **`y`** — run it
- **`n`** — skip
- **`s`** — same as no
- **`q`** — quit

Default depends on the recipe's safety class (`safe` defaults `y`, everything else defaults `n`).

## Share a debug bundle

```sh
envdoctor scan --bundle ./bundle.json
```

Writes a redacted JSON file — usernames, home directories, and absolute paths are scrubbed by default. Attach to a GitHub issue. The maintainer reads it back with:

```sh
envdoctor explain ./bundle.json
```

…and sees the same finding list you saw, plus the recipe hash so they can tell whether their envdoctor would have produced the same advice.
