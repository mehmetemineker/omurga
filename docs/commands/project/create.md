---
layout: default
title: project create
description: Create an Omurga project scaffold.
---

# `omurga project create`

## Use it when

You want a new project directory with a starter `omurga.yaml` and an
`environments` directory.

## Scenario

Create a project workspace on your development machine:

```bash
mkdir -p ~/omurga-lab
omurga project create demo --directory ~/omurga-lab
cd ~/omurga-lab/demo
```

The command creates files locally and does not deploy anything.
