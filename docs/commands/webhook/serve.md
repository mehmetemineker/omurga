---
layout: default
title: webhook serve
description: Run the signed image webhook listener.
---

## Use it when

You are testing webhook delivery manually or running the listener under a
custom process manager.

```bash
sudo omurga webhook serve
sudo omurga webhook serve --listen 127.0.0.1:8090
```

For normal host operation, use [webhook install]({{ '/commands/webhook/install/' | relative_url }}) to manage the
systemd service.
