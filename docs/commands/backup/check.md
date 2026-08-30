---
layout: default
title: backup check
description: Verify Restic repository integrity.
---

# `omurga backup check`

## Scenario

Run a repository check after configuring a new off-host destination:

```bash
sudo omurga backup check ./demo \
  --password-file /etc/omurga/backup/demo.password
```

This checks the repository and does not restore or delete project data.
