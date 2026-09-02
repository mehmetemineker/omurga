---
layout: default
title: ai plan
---

# `omurga ai plan`

Generate and validate an Omurga project manifest without writing project files.

```bash
omurga ai plan \
  "Create an HTTPS project for example.com using ghcr.io/acme/web:1.2.3 on port 3000" \
  --api-key-file ~/.config/omurga/llm-api-key
```

For longer requests:

```bash
omurga ai plan --prompt-file ./deployment-request.txt
```

The output is YAML by default or JSON with `--json`. A response is rejected if
it is not a valid version 1 manifest or contains secret-like environment
values.
