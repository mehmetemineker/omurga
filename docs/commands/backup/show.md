---
layout: default
title: backup show
description: Show details for a Restic snapshot.
---

# `omurga backup show`

```bash
sudo omurga backup show latest ./demo \
  --password-file /etc/omurga/backup/demo.password
```

Use the snapshot ID returned by `backup list` when `latest` is not specific
enough.
