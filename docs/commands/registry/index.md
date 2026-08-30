---
layout: default
title: Registry commands
description: Configure Docker registry profiles without exposing passwords.
---

# Registry commands

| Goal | Command |
| --- | --- |
| Save registry metadata | [registry add](add/) |
| List profiles | [registry list](list/) |
| Log in securely | [registry login](login/) |
| Remove a profile | [registry remove](remove/) |

## Scenario: private image registry

```bash
omurga registry add ghcr ghcr.io --username my-user
sudo omurga registry login ghcr --password-file /root/ghcr-password
omurga registry list
```

The password is read from a protected file or standard input.
