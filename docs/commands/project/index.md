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
| Deploy a project | [deploy](lifecycle/deploy/) |
| Repair a deployment | [repair](lifecycle/repair/) |
| Show project status | [status](lifecycle/status/) |
| Restart project services | [restart](lifecycle/restart/) |
| Stop project services | [stop](lifecycle/stop/) |
| Roll back a deployment | [rollback](lifecycle/rollback/) |
| Read service logs | [logs](access/logs/) |
| Execute a service command | [exec](access/exec/) |
| Open a service shell | [shell](access/shell/) |
| Inspect project state | [inspect](inspection/inspect/) |
| Compare revisions | [diff](inspection/diff/) |
| List deployments | [list](list/) |
| Delete a project | [delete](lifecycle/delete/) |

## Recommended workflow

```bash
omurga project create demo
omurga project validate ./demo --env production
omurga project render ./demo --env production --kind compose
omurga project diff ./demo --env production
sudo omurga --dry-run project deploy ./demo --env production
sudo omurga project deploy ./demo --env production
sudo omurga project status ./demo --env production
sudo omurga --dry-run project delete ./demo --env production
sudo omurga project delete ./demo --env production
```

Use `--env production` whenever the project has an environment overlay. Keep
passwords and tokens in the [secret commands](../secret/), not in the
manifest or environment files.

Project deletion preserves persistent data by default. To permanently remove
the project data, both explicit flags are required:

```bash
sudo omurga --yes project delete ./demo --env production --purge-data
```
