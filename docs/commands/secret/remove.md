---
layout: default
title: secret remove
description: Remove an encrypted project secret.
---

# `omurga secret remove`

## Use it when

A project no longer references a credential and it should be removed from the
encrypted store.

```bash
sudo omurga --env production secret remove old-token ./demo
sudo omurga --env production secret list ./demo
```

Confirm that the manifest no longer references the secret before removing it.
