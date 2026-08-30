---
layout: default
title: PostgreSQL commands
description: Operate project-scoped PostgreSQL instances safely.
---

# PostgreSQL commands

These commands target a PostgreSQL dependency declared in the project
manifest. If multiple instances exist, select one with `--instance`.

| Goal | Command |
| --- | --- |
| Check the instance | [postgres status](status/) |
| List databases | [postgres databases](databases/) |
| Create a database | [postgres create-db](create-db/) |
| Create a login role | [postgres create-user](create-user/) |
| Open `psql` | [postgres shell](shell/) |
| Create a dump | [postgres backup](backup/) |
| Restore a dump | [postgres restore](restore/) |

## Scenario: provision a database

```bash
sudo omurga postgres status ./demo
sudo omurga postgres create-db app ./demo
sudo omurga postgres create-user app_user ./demo --password-file /root/app-user-password
sudo omurga postgres databases ./demo
```
