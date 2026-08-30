---
layout: default
title: Getting started
description: Install Omurga on Debian or Ubuntu and deploy your first Docker project.
---

# Getting started

This guide takes you from a clean Debian or Ubuntu host to a running Omurga
project. The same workflow works on a Raspberry Pi running 64-bit Debian.

## Requirements

- Debian 11, 12, or 13, or Ubuntu 22.04, 24.04, or 26.04.
- `amd64` or `arm64` architecture.
- A user with `sudo` access.
- SSH access if the host is remote.
- At least 1 GB of available memory for the base host. Monitoring services require additional memory.

Check a host before installing:

```bash
cat /etc/os-release
dpkg --print-architecture
uname -m
systemctl --version | head -n 1
```

## Install from a GitHub release

Download the asset matching the host architecture from the repository’s
[Releases](https://github.com/mehmetemineker/omurga/releases) page.

```bash
VERSION=0.1.0

# Raspberry Pi and other 64-bit ARM hosts
curl -fL -o /tmp/omurga.deb \
  "https://github.com/mehmetemineker/omurga/releases/download/v${VERSION}/omurga_${VERSION}_arm64.deb"

# Intel or AMD hosts use this filename instead:
# curl -fL -o /tmp/omurga.deb \
#   "https://github.com/mehmetemineker/omurga/releases/download/v${VERSION}/omurga_${VERSION}_amd64.deb"

sudo apt install /tmp/omurga.deb
omurga version
```

The release also contains standalone binaries and `SHA256SUMS` for users who
do not want to install a Debian package:

```bash
sha256sum -c SHA256SUMS --ignore-missing
sudo install -m 0755 omurga-linux-arm64 /usr/local/bin/omurga
```

## Initialize the host

Inspect the detected platform first:

```bash
omurga host detect
```

Preview everything that initialization would change:

```bash
sudo omurga --dry-run host init
```

Apply initialization. This creates Omurga directories and installs Docker,
Caddy, and Restic:

```bash
sudo omurga host init
```

Install only selected components when the host already has some dependencies:

```bash
sudo omurga host init --skip-docker
sudo omurga host init --skip-caddy
sudo omurga host init --skip-restic
sudo omurga host install docker
sudo omurga host install caddy
sudo omurga host install restic
```

If a distribution Docker package conflicts with Docker CE, explicitly allow
Omurga to remove the conflicting packages:

```bash
sudo omurga host install docker --replace-conflicting-docker
```

Run the host doctor after initialization:

```bash
sudo omurga doctor
```

## Create and deploy a project

```bash
mkdir -p ~/omurga-lab
cd ~/omurga-lab

omurga project create demo
cd demo
omurga project validate
omurga project render --kind compose
omurga project render --kind caddy

# Preview the deployment
sudo omurga --dry-run project deploy .

# Deploy
sudo omurga project deploy .
sudo omurga project status .
```

The generated gateway ports bind to `127.0.0.1`. A Caddy route such as
`demo.localhost` is useful for local testing:

```bash
curl -i -H 'Host: demo.localhost' http://127.0.0.1
```

For a public domain, configure DNS and set `https: true` in the environment
overlay. See [Projects and environments](projects.md).

## Raspberry Pi workflow

On a 64-bit Raspberry Pi running Debian:

```bash
dpkg --print-architecture   # should be arm64
sudo omurga host init
sudo omurga doctor
```

The monitoring stack is optional and uses four containers. On a Pi 3, start
with the default host monitoring alerts and install Prometheus/Grafana only if
you have enough memory and disk:

```bash
sudo omurga monitoring install
sudo omurga monitoring status
```

## Progress, dry-run, and machine-readable output

Long operations show a spinner in an interactive terminal and stable activity
messages over SSH or when output is redirected:

```bash
sudo omurga --progress auto host update
sudo omurga --progress plain project deploy ./demo
sudo omurga --progress off backup create ./demo
```

`--dry-run` prints the planned commands and file changes without modifying the
host. It is safe to use while learning a new command:

```bash
sudo omurga --dry-run host update
sudo omurga --dry-run project deploy ./demo
sudo omurga --dry-run backup create ./demo --repository s3:s3.amazonaws.com/example
```

Use `--json` for automation and `--quiet` when only the exit code matters.
`--json`, `--quiet`, and `--dry-run` disable progress output automatically.
