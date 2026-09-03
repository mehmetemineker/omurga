---
layout: default
title: host add
description: Add or update an SSH host profile.
---

# `omurga host add`

```bash
omurga host add production 203.0.113.10 \
  --user deploy --identity ~/.ssh/id_ed25519
omurga host add production 203.0.113.10 --user deploy --port 2222
```

The profile does not store an SSH password.
