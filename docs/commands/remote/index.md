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

## Scenario: Raspberry Pi control from a workstation

```bash
omurga host add pi 192.168.0.50 --user mehmet --identity ~/.ssh/id_ed25519
omurga host list
omurga --host pi doctor
omurga --host pi project status ./demo
```

The Omurga binary must already be installed on the remote host.
