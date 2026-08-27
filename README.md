# Omurga

Omurga is a CLI for provisioning Ubuntu hosts, running Docker Compose projects,
configuring a Caddy gateway, and managing PostgreSQL, Redis, backups, secrets,
and health operations.

The project is in early development. The `version: 1` project manifest
validation layer, local host initialization, official Docker and Caddy package
installation, APT updates, and basic doctor checks are currently available.
Other commands expose the planned CLI contract and return an explicit error
until implemented.

## Requirements

- Go 1.23 or later

## Development

```bash
go mod tidy
go test ./...
go build ./cmd/omurga
```

Validate the example manifest:

```bash
go run ./cmd/omurga project validate ./examples/basic
go run ./cmd/omurga project validate ./examples/basic --env production
go run ./cmd/omurga --json project validate ./examples/basic --env production
```

Create a project scaffold and render runtime artifacts:

```bash
omurga project create demo
omurga project validate ./demo --env production
omurga project render ./demo --env production
omurga project render ./demo --env production --kind caddy
omurga project render ./demo --env production --output ./compose.generated.yaml
```

Rendered gateway ports bind to `127.0.0.1` only. Caddy is the public entry
point. `project render` uses deterministic preview ports and does not create
operational state. The deployment resolver uses SQLite for stable,
collision-safe port allocation across all managed projects. Dry-run planning
can inspect an existing database through a read-only connection.

On a supported Ubuntu host:

```bash
sudo omurga host init --dry-run
sudo omurga host init
sudo omurga host install docker --dry-run
sudo omurga host install docker
sudo omurga host install caddy
sudo omurga host install all
sudo omurga host update --dry-run
sudo omurga host update
omurga doctor
omurga doctor --json
```

`host update` uses `apt-get upgrade` by default. Use `--full` explicitly when a
`full-upgrade` is intended. Doctor exits with `0` for a healthy host, `1` when
warnings are present, and `2` when critical checks fail.

`host init` installs Docker and Caddy by default. Use `--skip-docker` or
`--skip-caddy` to opt out. Omurga does not automatically remove distribution
Docker packages that conflict with Docker CE. Inspect the dry-run output and use
`--replace-conflicting-docker` explicitly when replacement is intended.

See [docs/architecture-v1.md](docs/architecture-v1.md) for architectural decisions
and delivery milestones. See
[docs/project-manifest-v1.md](docs/project-manifest-v1.md) for the current
manifest and rendering contract, and [docs/state-v1.md](docs/state-v1.md) for
the operational state contract.
