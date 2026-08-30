---
layout: default
title: backup create
description: Create database dumps and a Restic snapshot.
---

# `omurga backup create`

## Use it when

You want an on-demand backup of project volumes and selected PostgreSQL or
Redis data.

```bash
sudo omurga --dry-run backup create ./demo \
  --repository s3:https://storage.googleapis.com/omurga-backups \
  --environment-file /etc/omurga/backup/gcs.env
sudo omurga backup create ./demo \
  --repository s3:https://storage.googleapis.com/omurga-backups \
  --password-file /etc/omurga/backup/demo.password \
  --environment-file /etc/omurga/backup/gcs.env
```

The example uses the Google Cloud Storage S3-compatible endpoint. Configure
the required HMAC credentials in the protected environment file. Run the
backup once manually before enabling the schedule.
