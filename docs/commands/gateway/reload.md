---
layout: default
title: gateway reload
description: Validate and reload Caddy.
---

# `omurga gateway reload`

## Use it when

Generated route files changed and Caddy must apply them.

```bash
sudo omurga --dry-run gateway reload
sudo omurga gateway reload
```

Omurga validates the complete Caddyfile before reloading it.
