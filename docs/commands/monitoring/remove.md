---
layout: default
title: monitoring remove
description: Remove the monitoring stack while preserving data by default.
---

# `omurga monitoring remove`

```bash
sudo omurga --dry-run monitoring remove
sudo omurga monitoring remove
sudo omurga --yes monitoring remove --purge-data
```

Without `--purge-data`, Prometheus and Grafana data is preserved.
