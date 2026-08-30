---
layout: default
title: host detect
description: Detect the supported Linux platform and providers.
---

# `omurga host detect`

## Use it when

You are connected to a new server and want to confirm that Omurga recognizes
its distribution, package manager, and service manager.

## Scenario

On a Raspberry Pi, run this before installing anything:

```bash
omurga host detect
omurga host detect --json
```

The JSON form is useful in an installation script. This command is read-only.
