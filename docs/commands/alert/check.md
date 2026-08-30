---
layout: default
title: alert check
description: Check host health and send alerts for state changes.
---

# `omurga alert check`

```bash
sudo omurga --dry-run alert check
sudo omurga alert check
sudo omurga alert check --channel telegram
```

Checks include resources, disk, container health, services, certificates, and
resource spikes. Repeated identical failures are not sent as new alerts.
