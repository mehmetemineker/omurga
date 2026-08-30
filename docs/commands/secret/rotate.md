---
layout: default
title: secret rotate
description: Replace an encrypted project secret.
---

# `omurga secret rotate`

## Use it when

A credential was changed or should be replaced as part of regular security
maintenance.

```bash
printf %s 'new-password' | sudo omurga --env production \
  secret rotate database-password ./demo --file -
sudo omurga project deploy ./demo --env production
```
