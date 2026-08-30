---
layout: default
title: Support commands
description: Create safe diagnostics for troubleshooting Omurga hosts.
---

# Support commands

| Goal | Command |
| --- | --- |
| Create a diagnostic archive | [support bundle](bundle/) |

## Scenario: collect information after a failed deployment

```bash
sudo omurga project status ./demo
sudo omurga project logs ./demo --service app --tail 200
sudo omurga support bundle --output /tmp/omurga-support.tar.gz
```

The archive is suitable for sharing: it excludes secret contents, environment
values, configuration files, and logs.
