---
layout: default
title: Webhook commands
description: Configure secure image-based deployments.
---

# Webhook commands

Webhooks react to a published immutable image digest. They do not deploy on
every Git commit; the image build and registry push happen first.

| Goal | Command |
| --- | --- |
| Create a target | [webhook add](add/) |
| List targets | [webhook list](list/) |
| Install the systemd service | [webhook install](install/) |
| Check the service | [webhook status](status/) |
| Run the listener manually | [webhook serve](serve/) |

## Scenario: deploy a published image

```bash
sudo omurga webhook add demo-production \
  --project demo --environment production --service app \
  --manifest /opt/omurga/projects/demo/omurga.yaml \
  --image-prefix ghcr.io/acme/demo
sudo omurga webhook install --binary /usr/local/bin/omurga
sudo omurga webhook status
```

Put the generated signing secret in the CI provider and expose the listener
through a TLS-terminating Caddy route.
