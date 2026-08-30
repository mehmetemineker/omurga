---
layout: default
title: alert test
description: Send a test alert through Telegram or email.
---

```bash
sudo omurga --dry-run alert test --channel telegram
sudo omurga alert test --channel telegram --message 'Omurga delivery test'
sudo omurga alert test --channel email
```

Use this after changing an alert credential file.
