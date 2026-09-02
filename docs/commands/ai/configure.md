---
layout: default
title: ai configure
---

# `omurga ai configure`

Save the endpoint and model for a remote OpenAI-compatible provider. The API
key is not accepted by this command and is never written to the configuration.

```bash
omurga ai configure \
  --endpoint https://api.example.com/v1/chat/completions \
  --model provider-model-name
```

The configuration is stored in the user Omurga configuration directory with
mode `0600`. Use `OMURGA_LLM_API_KEY`, `OMURGA_LLM_API_KEY_FILE`, or
`--api-key-file` with `ai plan` and `ai create` for authentication.
