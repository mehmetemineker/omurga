# Omurga

Omurga is a declarative CLI for provisioning supported Linux hosts, running Docker
Compose projects, configuring a Caddy gateway, and operating PostgreSQL, Redis,
automatic security updates, UFW, Fail2ban, backups, secrets, alerts, registries, shared services, and remote hosts.

The v1 command surface is implemented. Official host support covers Ubuntu
22.04, 24.04, and 26.04 plus Debian 11, 12, and 13. Host-changing commands
require root, support `--dry-run`, and can run through an SSH host profile
without an Omurga daemon.

## Progress output

Long-running commands report activity on standard error. Interactive terminals
show a spinner and elapsed time; redirected output and SSH sessions show stable
start and completion lines instead. Restic backup and restore additionally show
bytes, file counts, percentage, and an ETA when Restic provides it.

Use `--progress auto` (the default), `--progress tty`, `--progress plain`, or
`--progress off` to select the display. Progress is disabled automatically for
`--json`, `--quiet`, and `--dry-run`, so machine-readable and planned output
remain unchanged.

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

## Debian and Ubuntu packages

Debian and Ubuntu use the same Omurga package format. Build `amd64` and `arm64`
packages with the instructions in [packaging/deb/README.md](packaging/deb/README.md).
The `arm64` package can be installed on the tested Raspberry Pi system.

Tagged releases are built automatically by GitHub Actions. Each release
contains Linux `amd64` and `arm64` binaries, matching `.deb` packages, and a
`SHA256SUMS` file. Download the appropriate asset from the project’s GitHub
Releases page before installing it.

### Install on Debian or Ubuntu from a GitHub release

Check the architecture first:

```bash
dpkg --print-architecture
```

For a Raspberry Pi or another `arm64` host, download the `arm64` package. For
 an Intel or AMD host, download the `amd64` package:

```bash
curl -fL -o /tmp/omurga.deb \
  "https://github.com/mehmetemineker/omurga/releases/download/v0.2.0/omurga_0.2.0_arm64.deb"
sudo apt install /tmp/omurga.deb
```

For an `amd64` host, use this package name instead:

```bash
curl -fL -o /tmp/omurga.deb \
  "https://github.com/mehmetemineker/omurga/releases/download/v0.2.0/omurga_0.2.0_amd64.deb"
sudo apt install /tmp/omurga.deb
```

If you prefer the standalone binary on an `arm64` host:

```bash
curl -fL -o /tmp/omurga \
  "https://github.com/mehmetemineker/omurga/releases/download/v0.2.0/omurga-linux-arm64"
sudo install -m 0755 /tmp/omurga /usr/local/bin/omurga
```

Verify the installation:

```bash
omurga version
```

The package installs the CLI and documentation only. It does not provision
the host automatically. Run `sudo omurga host init` explicitly when host
provisioning is required.

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
sudo omurga project exec ./demo app nginx -T
sudo omurga project shell ./demo app
omurga project inspect ./demo --env production
omurga project diff ./demo --env production
sudo omurga project repair ./demo --env production
sudo omurga project rollback ./demo --env production
sudo omurga project delete ./demo --env production
```

Deploy waits for container health, validates generated Compose and Caddy
configuration, and preserves the previous healthy artifacts for rollback.
Stateless projects with a gateway use blue-green deployment with an inactive
Compose slot and ephemeral loopback ports; Caddy switches to the healthy slot
before the old slot is stopped. Stateful projects or projects with persistent
bind mounts use the safe in-place path with automatic rollback. Gateway ports
bind only to `127.0.0.1`; Caddy is the public entry point.
Persistent data is preserved during deletion unless both `--purge-data` and
`--yes` are supplied.

For troubleshooting, create a safe diagnostic archive that excludes secret
contents and environment values:

```bash
sudo omurga support bundle
```

HTTPS routes use Caddy automatic HTTPS and Let’s Encrypt-compatible ACME
certificate management. Point the domain DNS record to the host, allow inbound
TCP ports 80 and 443, and set the ACME account email in the manifest:

```yaml
gateway:
  email: ops@example.com
  routes:
    - domain: app.example.com
      service: app
      port: 80
      https: true
```

The certificate is requested during Caddy reload and renewed automatically.
Local domains such as `demo.localhost` cannot receive publicly trusted
certificates; keep those routes on HTTP with `https: false`.

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
sudo omurga host install ufw --ssh-port 22
sudo omurga host install unattended-upgrades
sudo omurga host install restic
sudo omurga --dry-run webhook install --binary /usr/local/bin/omurga
sudo omurga webhook install --binary /usr/local/bin/omurga
sudo omurga webhook status
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

## Prometheus and Grafana monitoring

Omurga can install a self-contained monitoring stack for host and container
metrics. The stack includes Prometheus, Grafana, Node Exporter, and cAdvisor.
Images are pinned and the official images support the Raspberry Pi `arm64`
platform.

```bash
sudo omurga monitoring install
sudo omurga monitoring status
```

Prometheus and Grafana bind to `127.0.0.1` by default. Access them through an
SSH tunnel:

```bash
ssh -L 3000:127.0.0.1:3000 user@server
```

Then open `http://127.0.0.1:3000`. Omurga creates a random Grafana admin
password in `/etc/omurga/monitoring/grafana-admin-password` on first install.
The password file is root-only. Custom ports or a LAN bind address can be
selected explicitly:

```bash
sudo omurga monitoring install --bind-address 192.168.0.50 \
  --prometheus-port 9090 --grafana-port 3000
```

The stack can be removed without deleting its data. Use the purge flag only
when the stored time series and Grafana data should be permanently deleted:

```bash
sudo omurga monitoring remove
sudo omurga --yes monitoring remove --purge-data
```

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
with `sudo omurga alert test --channel all`. Host monitoring checks disk usage,
failed services, and Caddy certificate expiry:

```bash
sudo omurga alert check
sudo omurga alert schedule
```

Configure CPU, memory, disk, certificate, and monitored-service thresholds in
the `monitor` section of `/etc/omurga/alerts.yaml`. Managed container health is
also checked. Resource spike detection compares CPU, memory, and disk usage with
a rolling baseline, requires consecutive samples, suppresses repeated alerts,
and sends a recovery notification when the issue is resolved.

## Documentation

The user-facing documentation is published at
[mehmetemineker.github.io/omurga](https://mehmetemineker.github.io/omurga/).

- [Architecture](docs/architecture-v1.md)
- [Platform providers](docs/platform-providers-v1.md)
- [Deploy a project end to end](docs/deploy-a-project.md)
- [Project manifest](docs/project-manifest-v1.md)
- [Project lifecycle](docs/project-lifecycle-v1.md)
- [Secrets](docs/secrets-v1.md)
- [Backups and alerts](docs/operations-v1.md)
- [Remote hosts](docs/remote-hosts-v1.md)
- [State database](docs/state-v1.md)
