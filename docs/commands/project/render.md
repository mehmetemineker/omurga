---
layout: default
title: project render
description: Render generated Docker Compose or Caddy configuration.
---

# `omurga project render`

## Use it when

You want to inspect the configuration Omurga will generate before deployment,
especially when debugging a gateway route or a published port.

```bash
omurga project render ./demo --kind compose
omurga project render ./demo --kind caddy
omurga project render ./demo --kind compose --output /tmp/demo-compose.yaml
```

This command renders to stdout by default and does not apply the result to the
host.
