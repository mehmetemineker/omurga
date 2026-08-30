---
layout: default
title: project status
description: Show deployment and live container status.
---

# `omurga project status`

## Use it when

You need to know whether a project is deployed and whether its containers are
running and healthy.

```bash
sudo omurga project status ./demo --env production
omurga project status ./demo --env production --json
```

If a project is unhealthy, continue with [logs]({{ '/commands/project/access/logs/' | relative_url }}) and
[repair]({{ '/commands/project/lifecycle/repair/' | relative_url }}).
