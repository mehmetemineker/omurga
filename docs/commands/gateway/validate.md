---
layout: default
title: gateway validate
description: Validate the complete Caddy configuration.
---

# `omurga gateway validate`

## Scenario

Run validation after editing a route or before a manual reload:

```bash
sudo omurga gateway validate
sudo omurga --dry-run gateway validate
```

Validation does not reload Caddy.
