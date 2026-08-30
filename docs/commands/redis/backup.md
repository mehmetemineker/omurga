---
layout: default
title: redis backup
description: Create a Redis RDB snapshot.
---

# `omurga redis backup`

## Scenario

Create a snapshot before maintenance:

```bash
sudo omurga redis backup ./demo --output /var/backups/demo-redis.rdb
```

Use the project backup workflow for scheduled off-host copies.
