---
layout: default
title: postgres create-user
description: Create a PostgreSQL login role.
---

# `omurga postgres create-user`

## Scenario

Read the new password from a root-only file instead of exposing it in shell
history:

```bash
sudo omurga postgres create-user app_user ./demo \
  --password-file /root/app-user-password
```

The password is sent to PostgreSQL through standard input.
