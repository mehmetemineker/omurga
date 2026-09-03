---
layout: default
title: project list
description: List deployments recorded by Omurga.
---

# `omurga project list`

## Use it when

You manage more than one project or environment and need a quick inventory of
recorded deployment revisions.

```bash
omurga project list
omurga project list --json
```

This reads the local Omurga state database. Use `--host production` to query the state
database on a configured remote host.
