---
layout: default
title: postgres backup
description: Create a PostgreSQL custom-format dump.
---

# `omurga postgres backup`

## Scenario

Create a dump before a risky schema change:

```bash
sudo omurga postgres backup ./demo --output /var/backups/demo.dump
```

Keep the dump in a protected location and include it in your off-host backup
workflow.
