---
layout: default
title: Project commands
description: Create, deploy, inspect, and operate Docker projects.
---

# Project commands

An Omurga project is a directory with `omurga.yaml` and optional environment
overlays. The manifest is the source of truth; deployment artifacts are
generated on the host.

| Task | Command page |
| --- | --- |
| Start a new manifest | [create](create/) |
| Check a manifest | [validate](validate/) |
| See the resolved configuration | [show](show/) |
| Preview Compose or Caddy | [render](render/) |
| Deploy or reconcile | [lifecycle](lifecycle/) |
| Run a command or shell | [service access](access/) |
| Investigate a project | [inspection](inspection/) |
| List deployments | [list](list/) |

## Recommended workflow

```bash
omurga project create demo
omurga project validate ./demo --env production
omurga project render ./demo --env production --kind compose
omurga project diff ./demo --env production
sudo omurga --dry-run project deploy ./demo --env production
sudo omurga project deploy ./demo --env production
sudo omurga project status ./demo --env production
```

Use `--env production` whenever the project has an environment overlay. Keep
passwords and tokens in the [secret commands](../secret/), not in the
manifest or environment files.
