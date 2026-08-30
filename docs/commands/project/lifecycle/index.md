---
layout: default
title: Project lifecycle commands
description: Deploy, repair, inspect status, stop, restart, roll back, and delete projects.
---

# Project lifecycle commands

These commands change or inspect the running state of a project.

| Situation | Command |
| --- | --- |
| Apply the desired manifest | [deploy](deploy/) |
| Reconcile a broken or incomplete deployment | [repair](repair/) |
| Check live containers | [status](status/) |
| Restart services | [restart](restart/) |
| Stop services | [stop](stop/) |
| Return to the previous healthy revision | [rollback](rollback/) |
| Remove a project | [delete](delete/) |

## Safe deployment sequence

```bash
omurga project validate ./demo --env production
omurga project diff ./demo --env production
sudo omurga --dry-run project deploy ./demo --env production
sudo omurga project deploy ./demo --env production
sudo omurga project status ./demo --env production
```

Deployments keep the previous healthy artifacts for rollback. Persistent data
is preserved when a project is deleted unless purge flags are explicitly used.
