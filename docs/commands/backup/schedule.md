---
layout: default
title: backup schedule
description: Enable periodic backups with a systemd timer.
---

# `omurga backup schedule`

## Scenario

Enable a daily project backup after a successful manual backup:

```bash
sudo omurga --dry-run backup schedule ./demo
sudo omurga backup schedule ./demo
sudo systemctl status omurga-backup-demo.timer
```

Use the project backup schedule from the manifest unless an explicit schedule
option is supplied.
