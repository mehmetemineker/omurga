---
layout: default
title: postgres restore
description: Restore a PostgreSQL custom-format dump.
---

# `omurga postgres restore`

## Scenario

Restore a tested dump during a maintenance window:

```bash
sudo omurga --yes postgres restore ./demo --file /var/backups/demo.dump
```

Omurga creates a pre-restore safety dump by default. Review the command help
for the available confirmation and safety options.
