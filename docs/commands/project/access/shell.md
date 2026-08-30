---
layout: default
title: project shell
description: Open an interactive shell in an active project service.
---

# `omurga project shell`

## Use it when

You need interactive investigation inside a running service.

```bash
sudo omurga project shell ./demo app
omurga --host pi project shell ./demo app
```

Omurga starts `/bin/sh` and allocates a TTY for a remote SSH execution. Exit
the shell with `exit` or `Ctrl-D`.
