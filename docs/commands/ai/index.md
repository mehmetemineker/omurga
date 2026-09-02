---
layout: default
title: AI project generation
description: Generate validated Omurga projects with a remote OpenAI-compatible LLM.
---

# AI project generation

The `ai` commands use a remote OpenAI-compatible API to generate an Omurga
manifest from a natural-language request. The model never executes commands.
The generated JSON is parsed, checked by the normal manifest validator, and
then written as YAML only by `ai create`.

Local models are not supported. API keys are never written to the Omurga AI
configuration file. Provide them through `OMURGA_LLM_API_KEY`,
`OMURGA_LLM_API_KEY_FILE`, or `--api-key-file`.

## Configure a remote provider

```bash
omurga ai configure \
  --endpoint https://api.example.com/v1/chat/completions \
  --model provider-model-name
```

This writes non-secret settings to the user configuration directory. The
endpoint must implement the OpenAI chat-completions request and response shape.

Store the API key outside the project directory:

```bash
mkdir -p ~/.config/omurga
install -m 600 /dev/null ~/.config/omurga/llm-api-key
nano ~/.config/omurga/llm-api-key
export OMURGA_LLM_API_KEY_FILE="$HOME/.config/omurga/llm-api-key"
```

Alternatively, pass the key file explicitly to each command:

```bash
omurga ai plan "Create an HTTPS project for example.com" \
  --api-key-file ~/.config/omurga/llm-api-key
```

## Generate a plan

`ai plan` calls the remote model, validates the returned manifest, and prints
the resulting YAML without creating files:

```bash
omurga ai plan \
  "Create a production Node.js service from ghcr.io/acme/web:1.2.3. \
   It listens on port 3000, uses HTTPS for example.com, and has a /health endpoint."
```

Use JSON for automation:

```bash
omurga --json ai plan "Create a production nginx service on port 80"
```

For a long prompt, use a file:

```bash
omurga ai plan --prompt-file ./deployment-request.txt
```

## Create a project

`ai create` writes `omurga.yaml` and a production environment overlay beneath
the selected parent directory. It refuses to overwrite an existing project:

```bash
omurga ai create \
  "Create a production project named lixy using ghcr.io/acme/lixy:2.0.0. \
   The app listens on port 3000, uses lixy.be with HTTPS, PostgreSQL, Redis, \
   daily backups, and a /health endpoint." \
  --directory ~/omurga-lab \
  --api-key-file ~/.config/omurga/llm-api-key
```

Override the name returned by the model when needed:

```bash
omurga ai create "Deploy the application described in this request" \
  --name lixy \
  --directory ~/omurga-lab
```

Review and validate the generated files before deployment:

```bash
cd ~/omurga-lab/lixy
omurga --env production project show .
omurga --env production project validate .
omurga --env production project render . --kind compose
omurga --env production project render . --kind caddy
sudo omurga --dry-run --env production project deploy .
sudo omurga --env production project deploy .
```

AI generation does not deploy automatically. Deployment remains an explicit
existing Omurga operation so a generated plan can be reviewed first.

## Safety rules

- Do not place passwords, tokens, or private keys in the prompt.
- Do not put API keys in `omurga.yaml` or environment overlays.
- Use `omurga secret set` for project secrets after generation.
- Review image names, exposed ports, gateway domains, dependencies, and backup
  destinations before running `project deploy`.
- Free remote API tiers can impose rate limits or change availability; the
  provider endpoint and model are configurable.
