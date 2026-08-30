---
layout: default
title: Alert commands
description: Configure and operate Telegram and email host alerts.
---

# Alert commands

| Goal | Command |
| --- | --- |
| Run a health check | [alert check](check/) |
| Send a delivery test | [alert test](test/) |
| Inspect safe configuration | [alert status](status/) |
| Enable periodic checks | [alert schedule](schedule/) |
| Disable periodic checks | [alert unschedule](unschedule/) |

## Scenario: verify Telegram alerts

```bash
sudo omurga alert status
sudo omurga alert test --channel telegram --message 'Omurga test'
sudo omurga alert schedule
sudo systemctl status omurga-alert-monitor.timer
```

Credential values are referenced from protected files.
