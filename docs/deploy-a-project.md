---
layout: default
title: Deploy a project end to end
description: Set up a Debian or Ubuntu host and deploy a Docker project with Omurga.
---

# Deploy a project end to end

This page shows the complete path from a Debian or Ubuntu host to a running
Docker project managed by Omurga. The example uses Nginx, but the same workflow
applies to any image that exposes a health-checkable HTTP service.

The commands assume:

- a supported Debian or Ubuntu host;
- a user with `sudo` access;
- `amd64` or `arm64` architecture; and
- DNS pointing to the host when a public domain is used.

## 1. Install Omurga

Choose the release version and repository, then download the package matching
the host architecture:

```bash
VERSION=0.1.0
REPO="mehmetemineker/omurga"

dpkg --print-architecture

# Use arm64 for a 64-bit Raspberry Pi or another ARM64 host.
curl -fL -o /tmp/omurga.deb \
  "https://github.com/${REPO}/releases/download/v${VERSION}/omurga_${VERSION}_arm64.deb"

# On Intel or AMD, use this package instead:
# curl -fL -o /tmp/omurga.deb \
#   "https://github.com/${REPO}/releases/download/v${VERSION}/omurga_${VERSION}_amd64.deb"

sudo apt install /tmp/omurga.deb
omurga version
```

Use the exact version and asset name shown on the [GitHub Releases
page](https://github.com/mehmetemineker/omurga/releases).

## 2. Prepare the Linux host

Preview the host changes first, then apply them:

```bash
sudo omurga --dry-run host init
sudo omurga host init
sudo omurga doctor
```

`host init` creates Omurga directories and installs or configures Docker,
Docker Compose, Caddy, Restic, automatic security updates, UFW, and Fail2ban.
It also configures Docker log rotation. Allow inbound TCP ports 80 and 443 on
the network firewall when the project will be public.

Check the key services:

```bash
sudo systemctl is-active docker
sudo systemctl is-active caddy
sudo ufw status
```

## 3. Create the project

Create a project directory and scaffold:

```bash
mkdir -p ~/omurga-lab
cd ~/omurga-lab
omurga project create demo
cd demo
nano omurga.yaml
```

Replace the generated `omurga.yaml` contents with this minimal Nginx project:

```yaml
version: 1
name: demo

services:
  app:
    image: nginx:1.27-alpine
    expose:
      - 80
    healthcheck:
      command: ["CMD-SHELL", "wget -q --spider http://127.0.0.1/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      startPeriod: 5s

gateway:
  routes:
    - domain: demo.localhost
      service: app
      port: 80
      https: false
```

For a public deployment, change the route to a real DNS name and enable HTTPS:

Edit the existing `gateway` block rather than adding a second `gateway` block:

```yaml
gateway:
  email: ops@example.com
  routes:
    - domain: demo.example.com
      service: app
      port: 80
      https: true
```

Point `demo.example.com` to the server before deploying. Caddy obtains and
renews the certificate automatically after the route is installed.

## 4. Validate and preview

Run validation and inspect both generated artifacts:

```bash
omurga project validate .
omurga project render . --kind compose
omurga project render . --kind caddy
```

Preview the host-side deployment without changing Docker, Caddy, or project
state:

```bash
sudo omurga --dry-run project deploy .
```

## 5. Deploy the project

Apply the deployment:

```bash
sudo omurga project deploy .
```

Omurga pulls the image, generates Compose and Caddy configuration, starts the
container, waits for its health check, validates the gateway, and reloads Caddy
only after the new route is valid. The previous healthy revision is retained
for rollback.

Inspect the result:

```bash
sudo omurga project status .
sudo omurga project logs . --service app --tail 100
sudo omurga gateway status
```

For the local example, test the route through Caddy rather than the container's
internal port:

```bash
curl -i -H 'Host: demo.localhost' http://127.0.0.1
```

For the public example, test the domain after DNS and certificate issuance:

```bash
curl -I https://demo.example.com
```

## 6. Operate the project

The usual lifecycle commands are:

```bash
sudo omurga project restart .
sudo omurga project stop .
sudo omurga project deploy .
sudo omurga project rollback .
sudo omurga project delete .
```

Deletion preserves persistent data by default. Permanent data removal requires
both explicit flags:

```bash
sudo omurga --yes project delete . --purge-data
```

Run the host health check whenever you need a complete system overview:

```bash
sudo omurga doctor
```

If deployment fails, inspect the project and service logs before retrying:

```bash
sudo omurga project status .
sudo omurga project logs . --service app --tail 200
sudo journalctl -u docker --since "15 minutes ago"
sudo journalctl -u caddy --since "15 minutes ago"
```
