---
layout: default
title: Monitoring commands
description: Install and operate Prometheus, Grafana, Node Exporter, and cAdvisor.
---

# Monitoring commands

| Goal | Command |
| --- | --- |
| Install the monitoring stack | [monitoring install](install/) |
| See monitoring containers | [monitoring status](status/) |
| Remove the stack | [monitoring remove](remove/) |

## Scenario: add monitoring to a Pi

```bash
sudo omurga --dry-run monitoring install
sudo omurga monitoring install
sudo omurga monitoring status
```

The stack uses additional memory and disk. On a Pi 3, check available
resources before installing it.
