---
layout: default
title: project restart
description: Restart the services of a deployed project.
---

# `omurga project restart`

## Use it when

The application is temporarily stuck or a process needs to be restarted
without changing its image, manifest, or persistent data.

```bash
sudo omurga --dry-run project restart ./demo --env production
sudo omurga project restart ./demo --env production
```

Use `deploy` when the desired configuration changed.
