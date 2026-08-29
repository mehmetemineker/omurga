# Omurga platform providers v1

## Support matrix

Omurga currently provides official provisioning support for:

| Distribution | Versions | Package manager | Service manager |
| --- | --- | --- | --- |
| Ubuntu | 22.04, 24.04, 26.04 | APT | systemd |
| Debian | 11, 12, 13 | APT | systemd |

Omurga matches the exact `ID` and `VERSION_ID` values from `/etc/os-release`.
It does not automatically treat a derivative as Ubuntu or Debian merely because
`ID_LIKE` contains one of those names. This prevents a repository or package
contract from being applied to an untested distribution.

Use `omurga host detect` to inspect the selected provider and its support level.
Unsupported distributions fail before host files or packages are changed.

## Extension model

Platform-specific behavior is split into three contracts:

- `DistributionProvider` validates a release and supplies Docker and Caddy
  repository, prerequisite, conflict, and package specifications.
- `PackageManager` supplies refresh, upgrade, install, remove, package query,
  and architecture operations.
- `ServiceManager` supplies service health, enable, disable, reload, daemon
  reload, and timer inspection operations.

The CLI, installer, doctor, gateway, project lifecycle, and backup scheduling
commands consume these contracts instead of embedding APT or systemd commands.
The current scheduled-job artifact writer produces systemd units, so a provider
without systemd will also require a scheduler artifact implementation. Ubuntu
and Debian share the APT/systemd implementations but produce distinct official
Docker repository URLs and release suites.

## Adding another distribution

Adding a distribution requires:

1. Implement a `DistributionProvider` for the exact `/etc/os-release` ID.
2. Implement or reuse the required `PackageManager` and `ServiceManager`.
3. Define Docker and Caddy component specifications using that distribution's
   official repositories and package names.
4. Register the provider in `DefaultProviderRegistry`.
5. Add detection, unsupported-version, dry-run, repository, installation, and
   doctor tests.
6. Validate installation and lifecycle operations on real supported releases
   before marking the provider as official.

Package and service managers are intentionally separate from the distribution
provider. A future RPM-based provider can therefore introduce DNF while reusing
systemd. A non-systemd provider can supply another service implementation
without changing project deployment or gateway reconciliation, while scheduled
jobs additionally require a matching scheduler artifact writer.
