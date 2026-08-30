# Changelog

## 0.2.0

### Added

- Prometheus, Grafana, Node Exporter, and cAdvisor installation and lifecycle commands.
- Host and managed-container resource monitoring with CPU, memory, disk, health, and resource-spike alerts.
- Telegram and email alert checks, tests, scheduling, and resolution tracking.
- Signed, replay-protected image deployment webhooks with automatic systemd installation.
- Automatic rollback and zero-downtime deployment for eligible stateless projects.
- Debian and Ubuntu host hardening with UFW, Fail2ban, automatic security updates, and Docker log rotation.
- Project management commands: `exec`, `shell`, `inspect`, `diff`, and `repair`.
- Secrets-free `support bundle` diagnostics.
- Scenario-based command documentation with separate pages for command categories and commands.

### Changed

- Release artifacts are built for Linux `amd64` and `arm64` as standalone binaries and Debian packages.
- Project gateway routes use generated Caddy snippets with validated reloads.
- Backup workflows support scheduled snapshots, retention, database dumps, and off-host repositories.

