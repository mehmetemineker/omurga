# Omurga

Omurga is a declarative CLI for provisioning supported Linux hosts, running Docker
Compose projects, configuring a Caddy gateway, and operating PostgreSQL, Redis,
backups, secrets, alerts, registries, shared services, and remote hosts.

The v1 command surface is implemented. Official host support covers Ubuntu
22.04, 24.04, and 26.04 plus Debian 11, 12, and 13. Host-changing commands
require root, support `--dry-run`, and can run through an SSH host profile
without an Omurga daemon.

## Development

Omurga requires Go 1.23 or later:

```bash
go mod tidy
go test ./...
go build ./cmd/omurga
```

If Go is not installed locally, use the official container:

```bash
docker run --rm -v "$PWD:/workspace" -w /workspace golang:1.23 \
  sh -c "go test ./... && go build -o build/omurga ./cmd/omurga"
```

On PowerShell, replace `$PWD` with `${PWD}`. Most host operations need a real
supported Linux VM; manifest validation, rendering, scaffolding, and unit tests
work on Windows, macOS, and Linux.

## Project workflow

```bash
omurga project create demo
omurga project validate ./demo --env production
omurga project show ./demo --env production
omurga project render ./demo --env production --kind compose
omurga --dry-run project deploy ./demo --env production
sudo omurga project deploy ./demo --env production
sudo omurga project status ./demo --env production
sudo omurga project logs ./demo --env production --follow
sudo omurga project rollback ./demo --env production
sudo omurga project delete ./demo --env production
```

Deploy waits for container health, validates generated Compose and Caddy
configuration, and preserves the previous healthy artifacts for rollback.
Gateway ports bind only to `127.0.0.1`; Caddy is the public entry point.
Persistent data is preserved during deletion unless both `--purge-data` and
`--yes` are supplied.

Environment overlays are regular YAML files under `environments/`. Non-secret
service values can also be edited through the CLI:

```bash
omurga env list ./demo
omurga env set production app LOG_LEVEL warning ./demo
omurga env unset production app LOG_LEVEL ./demo
```

## Secrets

Secret values are accepted only from a file or standard input, encrypted with
age, and never written to generated Compose files or normal command output:

```bash
printf %s 'change-me' | sudo omurga --env production secret set database-password ./demo --file -
sudo omurga --env production secret list ./demo
printf %s 'replacement' | sudo omurga --env production secret rotate database-password ./demo --file -
```

The private age identity is stored at `/etc/omurga/keys/identity.agekey`.
Encrypted project stores live under `/etc/omurga/secrets`; deploy materializes
only required values under `/run/omurga/secrets` with manifest UID, GID, and
mode settings.

## Host provisioning and remote hosts

```bash
omurga host detect
sudo omurga host init --dry-run
sudo omurga host init
sudo omurga host update
sudo omurga host install all
omurga doctor
```

Distribution-specific behavior is selected from `/etc/os-release`. Provisioning
is implemented through distribution, package-manager, and service-manager
interfaces so additional Linux families can be added without rewriting command
or project lifecycle code. See the [platform provider guide](docs/platform-providers-v1.md).

Remote profiles are user-local and contain no SSH passwords:

```bash
omurga host add production 203.0.113.10 --user deploy --identity ~/.ssh/id_ed25519
omurga host list
omurga --host production doctor
omurga --host production --dry-run host update
```

The Omurga binary must already be available on the remote host. By default,
remote commands use `sudo -n`; disable it per profile with `--sudo=false`.

## PostgreSQL, Redis, and shared services

```bash
sudo omurga --env production postgres databases ./demo
sudo omurga --env production postgres backup ./demo --output ./database.dump
sudo omurga --env production --yes postgres restore ./demo --file ./database.dump
sudo omurga --env production redis stats ./demo
sudo omurga service catalog
sudo omurga service install redis --name cache
```

PostgreSQL restore creates a pre-restore safety dump by default. Shared-service
removal preserves its bind-mounted data unless explicit purge flags are used.

## Restic backups and alerts

Backups use Restic and support local, SFTP, S3-compatible, and other Restic
repository URIs. Repository passwords and backend credentials must be stored in
root-only files.

```bash
sudo omurga --env production backup create ./demo \
  --repository sftp:backup@example.net:/srv/restic/demo \
  --password-file /etc/omurga/backup/demo.password --init
sudo omurga --env production backup list ./demo --password-file /etc/omurga/backup/demo.password
sudo omurga --env production backup schedule ./demo --password-file /etc/omurga/backup/demo.password
sudo omurga --env production --yes backup prune ./demo --password-file /etc/omurga/backup/demo.password
```

Telegram and SMTP settings are read from `/etc/omurga/alerts.yaml`. Tokens and
passwords are referenced through credential files. Test configured channels
with `sudo omurga alert test --channel all`.

## Documentation

- [Architecture](docs/architecture-v1.md)
- [Platform providers](docs/platform-providers-v1.md)
- [Project manifest](docs/project-manifest-v1.md)
- [Project lifecycle](docs/project-lifecycle-v1.md)
- [Secrets](docs/secrets-v1.md)
- [Backups and alerts](docs/operations-v1.md)
- [Remote hosts](docs/remote-hosts-v1.md)
- [State database](docs/state-v1.md)
