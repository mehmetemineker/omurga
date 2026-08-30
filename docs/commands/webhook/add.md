---
layout: default
title: webhook add
description: Create a signed image deployment target.
---

# `omurga webhook add`

```bash
sudo omurga webhook add demo-production \
  --project demo --environment production --service app \
  --manifest /opt/omurga/projects/demo/omurga.yaml \
  --image-prefix ghcr.io/acme/demo
```

Save the printed secret immediately as a CI secret. It is not printed again by
`webhook list`.
