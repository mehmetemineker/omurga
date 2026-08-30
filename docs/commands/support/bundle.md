---
layout: default
title: support bundle
description: Create a secrets-free diagnostic archive.
---

# `omurga support bundle`

## Use it when

You need to send host diagnostics while keeping credentials and application
configuration private.

```bash
sudo omurga --dry-run support bundle
sudo omurga support bundle
sudo omurga support bundle --output /tmp/omurga-support.tar.gz
sudo omurga support bundle --json
```

The bundle contains doctor results, deployment metadata, container status,
failed systemd units, and Caddy status. It refuses to overwrite an existing
output file.
