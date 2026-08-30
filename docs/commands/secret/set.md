---
layout: default
title: secret set
description: Create or replace an encrypted project secret.
---

# `omurga secret set`

## Use it when

A project needs a new password, token, or private value.

```bash
printf %s 'change-me' | sudo omurga --env production \
  secret set database-password ./demo --file -
sudo omurga --env production secret list ./demo
```

Do not pass the value as a positional argument because shell history may save
it. Use `--file <path>` or `--file -`.
