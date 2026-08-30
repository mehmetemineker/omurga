---
layout: default
title: Redis commands
description: Operate project-scoped Redis instances.
---

# Redis commands

Use these commands for a Redis dependency declared in the project manifest.

| Goal | Command |
| --- | --- |
| Check the instance | [redis status](status/) |
| View Redis statistics | [redis stats](stats/) |
| Open `redis-cli` | [redis shell](shell/) |
| Create an RDB snapshot | [redis backup](backup/) |
| Delete all Redis data | [redis flush](flush/) |

## Scenario: inspect a cache problem

```bash
sudo omurga redis status ./demo
sudo omurga redis stats ./demo
sudo omurga redis shell ./demo
```

`flush` is destructive. Use it only when the cache can be rebuilt.
