---
layout: default
title: ai create
---

# `omurga ai create`

Generate a manifest with a remote LLM and create a new project directory. The
command never overwrites an existing directory and never deploys automatically.

```bash
omurga ai create \
  "Create a production project named demo using ghcr.io/example/demo:2.0.0. \
   It listens on port 3000, uses HTTPS for demo.example.com, and has a /health endpoint." \
  --directory ~/omurga-lab \
  --api-key-file ~/.config/omurga/llm-api-key
```

This creates:

```text
~/omurga-lab/demo/omurga.yaml
~/omurga-lab/demo/environments/production.yaml
```

Review the generated files and deploy with the normal lifecycle:

```bash
cd ~/omurga-lab/demo
omurga --env production project validate .
sudo omurga --dry-run --env production project deploy .
sudo omurga --env production project deploy .
```

Use `--name` to override the project name returned by the model.
