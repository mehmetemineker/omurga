---
layout: default
title: Projects and environments
description: Define, validate, deploy, expose, and operate Docker projects with Omurga.
---

# Projects and environments

Omurga projects are small, reviewable YAML applications. The base manifest
describes the application; an environment overlay changes values such as image
tags, resource limits, and gateway domains.

## Create a project

```bash
omurga project create blog --directory ~/omurga-lab
cd ~/omurga-lab/blog
```

The scaffold contains `omurga.yaml` and an `environments/` directory. You can
also start from the maintained [basic example](https://github.com/mehmetemineker/omurga/tree/main/examples/basic).

## A complete manifest

This compact example shows the most important fields:

```yaml
version: 1
name: blog

services:
  app:
    image: ghcr.io/example/blog:1.4.0
    pullPolicy: if-not-present
    expose: [3000]
    environment:
      APP_ENV: development
      LOG_LEVEL: debug
      DATABASE_PASSWORD_FILE: /run/secrets/database_password
    secrets:
      - name: database-password
        target: /run/secrets/database_password
        mode: "0400"
        uid: 1000
        gid: 1000
    volumes:
      - name: app-data
        target: /app/data
    resources:
      cpus: "1.0"
      memory: 512M
      pids: 200
    logging:
      driver: json-file
      maxSize: 100M
      maxFiles: 5
    healthcheck:
      command: [CMD, wget, --spider, http://localhost:3000/health]
      interval: 30s
      timeout: 5s
      retries: 3
      startPeriod: 20s

gateway:
  email: ops@example.com
  routes:
    - domain: blog.example.com
      service: app
      port: 3000
      https: true

dependencies:
  postgres:
    type: postgres
    version: "16"
    mode: project
    database: blog
    user: blog
    passwordSecret: database-password
  redis:
    type: redis
    version: "7"
    mode: project
    persistence: aof
    maxMemory: 256M
    evictionPolicy: allkeys-lru
```

### Services

Each service needs an image. `expose` declares container ports that gateway
routes may reference; Omurga publishes them on a loopback-only host port.
`pullPolicy` accepts `always`, `if-not-present`, and `never`. Restart policies
are `no`, `always`, `on-failure`, and `unless-stopped`.

Environment values are plain configuration, not secrets. Use the secret store
for passwords, tokens, and keys. Volume names are managed under the project’s
persistent data directory.

Health checks are used during deployment. A service that never becomes healthy
causes the deployment to fail and triggers the rollback path when a previous
healthy revision exists.

### PostgreSQL and Redis dependencies

`mode: project` creates a database or Redis container inside the project’s
Compose application. PostgreSQL requires `database`, `user`, and
`passwordSecret`. Redis supports `persistence`, `maxMemory`, and
`evictionPolicy`.

`mode: shared` is reserved for shared service management and is not rendered as
a project dependency. Use `omurga service` for shared instances.

## Environments

Create an overlay at `environments/production.yaml`:

```yaml
host: production

services:
  app:
    image: ghcr.io/example/blog:1.4.3
    environment:
      APP_ENV: production
      LOG_LEVEL: warning

gateway:
  routes:
    - domain: blog.example.com
      service: app
      port: 3000
      https: true
```

List, inspect, and edit non-secret values with the CLI:

```bash
omurga env list ./blog
omurga env show production ./blog
omurga env set production app LOG_LEVEL warning ./blog
omurga env unset production app LOG_LEVEL ./blog
```

The global `--env production` option selects an overlay for commands that take
a project path:

```bash
omurga --env production project validate ./blog
sudo omurga --env production project deploy ./blog
```

## Secrets

Secrets are encrypted with age and stored outside the project working tree.
They are never placed in generated Compose files or normal command output.

```bash
printf %s 'replace-me' | sudo omurga --env production \
  secret set database-password ./blog --file -

sudo omurga --env production secret list ./blog

printf %s 'new-value' | sudo omurga --env production \
  secret rotate database-password ./blog --file -

sudo omurga --env production secret remove database-password ./blog
```

Use `--file /path/to/value` for a file or `--file -` for standard input. The
private identity is stored in `/etc/omurga/keys/identity.agekey`; encrypted
stores are under `/etc/omurga/secrets`.

## Validate and render

Validate before deploying:

```bash
omurga project validate ./blog
omurga --env production project validate ./blog
```

Inspect the generated artifacts without changing the host:

```bash
omurga --env production project render ./blog --kind compose
omurga --env production project render ./blog --kind caddy
omurga --env production project render ./blog --kind compose --output /tmp/blog-compose.yaml
```

The `compose` artifact is the Docker Compose document. The `caddy` artifact is
the project route imported by the host Caddyfile.

## Deploy, inspect, control

```bash
sudo omurga --env production project deploy ./blog
sudo omurga --env production project status ./blog
sudo omurga --env production project logs ./blog --tail 200
sudo omurga --env production project logs ./blog --service app --follow
sudo omurga --env production project restart ./blog
sudo omurga --env production project stop ./blog
```

Deployments validate the manifest, render Compose and Caddy, pull images, wait
for health, and reload Caddy only after configuration validation succeeds.
Stateless gateway projects use a blue-green path: the inactive slot starts on
temporary loopback ports, Caddy switches to it, and the old slot is stopped.
Stateful projects use the safe in-place path to avoid running two writers
against the same persistent data.

Inspect a deployment plan first:

```bash
sudo omurga --dry-run --env production project deploy ./blog
```

## Rollback and deletion

```bash
sudo omurga --env production project rollback ./blog
sudo omurga --dry-run --env production project rollback ./blog
sudo omurga --env production project delete ./blog
```

Deletion preserves persistent data. To delete it permanently, both flags are
required:

```bash
sudo omurga --yes --env production project delete ./blog --purge-data
```

List all recorded deployments or inspect the resolved manifest:

```bash
omurga project list
omurga --env production project show ./blog
```

## Caddy and HTTPS

For a public domain, point DNS to the server and allow inbound TCP ports 80 and
443. Set `gateway.email` and `https: true`:

```yaml
gateway:
  email: ops@example.com
  routes:
    - domain: app.example.com
      service: app
      port: 3000
      https: true
```

### Route response headers

Response headers are configured per gateway route. This is useful when a
preview route must hide proxy headers and prevent search engine indexing:

```yaml
gateway:
  email: ops@example.com
  routes:
    - domain: preview.blog.example.com
      service: app
      port: 3000
      https: true
      responseHeaders:
        remove:
          - Server
          - Via
        set:
          X-Robots-Tag: "noindex, nofollow, noarchive, nosnippet"
```

`remove` is applied to headers returned by the upstream service and to the
final Caddy response. `set` writes the configured response headers. Header
values are quoted safely in the generated Caddyfile, and values containing
carriage returns or line feeds are rejected during validation.

Environment overlays replace the complete `gateway.routes` list. Therefore,
when adding headers to only the preview environment, define the complete
preview route in `environments/preview.yaml`:

```yaml
services:
  app:
    image: ghcr.io/example/blog:preview

gateway:
  email: ops@example.com
  routes:
    - domain: preview.blog.example.com
      service: app
      port: 3000
      https: true
      responseHeaders:
        remove: [Server, Via]
        set:
          X-Robots-Tag: "noindex, nofollow, noarchive, nosnippet"
```

Validate and deploy the selected environment:

```bash
omurga --env preview project validate ./blog
sudo omurga --env preview project render ./blog --kind caddy
sudo omurga --env preview project deploy ./blog
curl -I https://preview.blog.example.com
```

`X-Robots-Tag` discourages indexing but does not protect the application from
visitors. Add authentication or network restrictions when preview content is
private.

Check the gateway independently:

```bash
sudo omurga gateway list
sudo omurga gateway status
sudo omurga gateway validate
sudo omurga gateway reload
```

Local names such as `demo.localhost` are useful for testing but cannot receive
publicly trusted Let’s Encrypt certificates. Use `https: false` locally.
