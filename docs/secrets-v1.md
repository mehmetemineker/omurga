---
layout: default
title: Secrets
---

# Secret management

Omurga uses one age X25519 identity per host. The private identity is generated
on the first `secret set` operation and stored at
`/etc/omurga/keys/identity.agekey` with mode `0600`. Backing up this identity is
mandatory: encrypted project stores cannot be recovered without it.

Each project and environment has one encrypted store:

```text
/etc/omurga/secrets/<project>/<environment>.age
```

The complete JSON payload is encrypted; names and values are not visible from
the persistent file. Values are supplied with `--file PATH` or `--file -` and
are never accepted as positional command arguments.

```bash
printf %s 'value' | sudo omurga --env production secret set api-token ./project --file -
sudo omurga --env production secret list ./project
sudo omurga --env production secret remove api-token ./project
```

During deploy, Omurga decrypts only secrets referenced by the resolved manifest
and writes them to `/run/omurga/secrets/<project>/<environment>`. Files are
atomically replaced, stale files are removed, and manifest mode, UID, and GID
are applied before Docker Compose starts. The runtime directory is expected to
be ephemeral and must not be backed up.

The same secret may be mounted by multiple services only when every mount uses
identical mode, UID, and GID settings. Omurga validates this before deployment.

Manual runtime files remain supported for migration when no encrypted store
exists. They must be regular files with no group or other permissions.
