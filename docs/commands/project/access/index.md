---
layout: default
title: Project service access
description: Inspect logs and run commands inside active project services.
---

# Project service access

| Need | Command |
| --- | --- |
| Read recent output | [logs](logs/) |
| Run one diagnostic command | [exec](exec/) |
| Open an interactive shell | [shell](shell/) |

## Unhealthy application scenario

```bash
sudo omurga project status ./demo --env production
sudo omurga project logs ./demo --env production --service app --tail 200
sudo omurga project exec ./demo app wget -qO- http://127.0.0.1/health
sudo omurga project shell ./demo app
```

Only services declared in the project manifest can be selected by `exec` and
`shell`.
