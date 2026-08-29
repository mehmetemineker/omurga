# Backups and alerts

## Restic repositories

`backup.destination` may contain a Restic repository URI, or it can be
overridden with `--repository`. SFTP and S3-compatible repositories use Restic's
native URI formats. The repository password is always read from a mode `0600`
file. S3 credentials can be supplied in a separate mode `0600` environment
file:

```text
AWS_ACCESS_KEY_ID=example
AWS_SECRET_ACCESS_KEY=example
```

```bash
sudo omurga backup create ./project --env production \
  --password-file /etc/omurga/backup/production.password \
  --environment-file /etc/omurga/backup/production.env
```

A project snapshot includes available generated artifacts, persistent volume
directories, the source manifest, encrypted secret store, and SQLite state.
PostgreSQL selections are staged with `pg_dump --format=custom`; Redis
selections are staged after a synchronous `SAVE`. A command succeeds only after
Restic confirms that the snapshot reached the repository.

`backup restore` restores into a staging target. Applying restored data to a
running project is deliberately separate. PostgreSQL's direct restore command
creates a custom-format safety dump before changing database objects unless
`--no-safety-backup` is explicitly set.

Retention defaults to 7 daily, 4 weekly, and 6 monthly snapshots when all
manifest values are zero. Pruning requires both `--yes` and an explicit command.

`backup schedule` writes an `omurga-backup-<project>-<environment>.service` and
timer under `/etc/systemd/system`. Timers are persistent and use a randomized
five-minute delay. The manifest schedule accepts `HH:MM` or a systemd calendar
expression.

## Alerts

Alert configuration is stored in `/etc/omurga/alerts.yaml`. Credential values
are referenced through root-only files:

```yaml
telegram:
  enabled: true
  tokenFile: /etc/omurga/telegram.token
  chatId: "-1001234567890"

smtp:
  enabled: true
  host: smtp.example.com
  port: 587
  username: alerts@example.com
  passwordFile: /etc/omurga/smtp.password
  from: alerts@example.com
  to:
    - ops@example.com
  tls: starttls
```

SMTP supports `starttls` and `implicit`; TLS 1.2 or newer is mandatory.
Telegram uses the HTTPS Bot API `sendMessage` method. `alert test` can target
`telegram`, `email`, or `all`. Project events listed under `alerts.on` trigger
best-effort notifications for deployment and backup failures.

