---
layout: default
title: monitoring install
description: Install or update the Prometheus and Grafana stack.
---

# `omurga monitoring install`

```bash
sudo omurga --dry-run monitoring install
sudo omurga monitoring install
sudo omurga monitoring install --grafana-admin-password-file /root/grafana-password
```

The stack includes Prometheus, Grafana, Node Exporter, and cAdvisor. The
password file must be protected and is not printed.
