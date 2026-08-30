---
layout: default
title: Secret commands
description: Store and manage encrypted project secrets.
---

# Secret commands

Secret values are encrypted with age and are materialized only for a
deployment. Omurga never prints secret contents.

| Goal | Command |
| --- | --- |
| Create or replace a secret | [secret set](set/) |
| List secret names | [secret list](list/) |
| Replace an existing secret | [secret rotate](rotate/) |
| Remove a secret | [secret remove](remove/) |

## Scenario: database password

```bash
printf %s 'change-me' | sudo omurga --env production \
  secret set database-password ./demo --file -
sudo omurga --env production secret list ./demo
sudo omurga --env production project deploy ./demo
```

The value is read from a file or standard input, not a command-line argument.
