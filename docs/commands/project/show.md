---
layout: default
title: project show
description: Show the resolved project manifest.
---

# `omurga project show`

## Use it when

You need to confirm what Omurga sees after merging the base manifest with an
environment overlay.

```bash
omurga project show ./demo --env production
omurga project show ./demo --env production --json
```

Do not use this command as a secret viewer. Secret values are stored separately
and are never printed by Omurga.
