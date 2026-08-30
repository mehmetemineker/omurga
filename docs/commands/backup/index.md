---
layout: default
title: Backup commands
description: Create, restore, prune, and schedule Omurga backups.
---

# Backup commands

Omurga combines database dumps with Restic snapshots and can send repositories
to destinations such as Google Cloud Storage through the configured backend.

| Goal | Command |
| --- | --- |
| Create a backup | [backup create](create/) |
| List snapshots | [backup list](list/) |
| Show snapshot details | [backup show](show/) |
| Verify repository integrity | [backup check](check/) |
| Restore to a staging target | [backup restore](restore/) |
| Apply retention | [backup prune](prune/) |
| Schedule periodic backups | [backup schedule](schedule/) |
| Remove a schedule | [backup unschedule](unschedule/) |

## Scenario: scheduled off-host backup

```bash
sudo omurga --dry-run backup create ./demo
sudo omurga backup create ./demo
sudo omurga backup list ./demo
sudo omurga backup schedule ./demo
sudo omurga backup prune ./demo
```

Configure the repository and credentials before scheduling. Keep repository
credentials in protected files, never in the manifest.
