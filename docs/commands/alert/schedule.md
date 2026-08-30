---
layout: default
title: alert schedule
description: Enable the periodic systemd host alert monitor.
---

```bash
sudo omurga --dry-run alert schedule
sudo omurga alert schedule
sudo omurga alert schedule --schedule 'hourly'
```

The monitor runs `alert check` through a systemd timer.
