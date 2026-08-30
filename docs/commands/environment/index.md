---
layout: default
title: Environment commands
description: Manage non-secret project environment overlays.
---

# Environment commands

Environment overlays let the same project manifest use different values for
development, staging, and production.

| Goal | Command |
| --- | --- |
| List available overlays | [env list](list/) |
| Read an overlay | [env show](show/) |
| Add or change a value | [env set](set/) |
| Remove a value | [env unset](unset/) |

## Scenario: production log level

```bash
omurga env list ./demo
omurga env set production app LOG_LEVEL info ./demo
omurga env show production ./demo
sudo omurga project deploy ./demo --env production
```

Never store passwords, API tokens, or private keys in an environment overlay.
Use [encrypted secrets](../secret/).
