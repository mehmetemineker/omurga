---
layout: default
title: webhook install
description: Install and enable the Omurga webhook systemd service.
---

# `omurga webhook install`

```bash
sudo omurga --dry-run webhook install --binary /usr/local/bin/omurga
sudo omurga webhook install --binary /usr/local/bin/omurga
sudo systemctl status omurga-webhook.service
```

The service listens on loopback by default. Put Caddy or another TLS proxy in
front of it before allowing internet traffic.
