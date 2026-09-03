---
layout: default
title: Remote host commands
description: Manage SSH host profiles and run Omurga remotely.
---

# Remote host commands

Remote profiles are stored locally and contain SSH connection metadata.

| Goal | Command |
| --- | --- |
| Add or update a profile | [host add](add/) |
| List profiles | [host list](list/) |
| Inspect a profile | [host show](show/) |
| Remove a profile | [host remove](remove/) |

## Scenario: Remote host control from a workstation

```bash
omurga host add production 203.0.113.10 --user deploy --identity ~/.ssh/id_ed25519
omurga host list
omurga --host production doctor
omurga --host production project status ./demo
```

The Omurga binary must already be installed on the remote host.
