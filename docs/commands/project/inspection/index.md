---
layout: default
title: Project inspection commands
description: Inspect project configuration and compare desired and active revisions.
---

# Project inspection commands

Use these read-only commands before changing a deployment or while diagnosing
one.

- [inspect](inspect/) shows a safe configuration and deployment summary.
- [diff](diff/) compares the desired manifest revision with the active revision.

## Investigation scenario

```bash
omurga project inspect ./demo --env production
omurga project diff ./demo --env production
sudo omurga project status ./demo --env production
```
