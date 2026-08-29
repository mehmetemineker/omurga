# Remote host management

Remote execution uses OpenSSH and does not require a resident Omurga daemon.
Profiles are stored in the current user's platform configuration directory as
`omurga/hosts.yaml` with mode `0600`. Profiles contain an address, user, port,
private-key path, remote Omurga path, and sudo preference. They never contain an
SSH password or private-key material.

```bash
omurga host add production server.example.com \
  --user deploy --port 22 --identity ~/.ssh/id_ed25519
omurga host show production
omurga --host production doctor
omurga --host production --env production project status /srv/apps/blog
```

The remote host must have Omurga installed. Commands use `sudo -n` by default,
so the SSH user needs an appropriate non-interactive sudo policy for privileged
operations. OpenSSH performs host-key verification using the user's normal SSH
configuration and known-hosts files.

Arguments after remote forwarding refer to the remote filesystem. A local
manifest path is therefore not uploaded automatically; deploy a manifest that
already exists on the target host or synchronize it through the user's normal
delivery workflow.

Interactive PostgreSQL and Redis shells allocate a terminal. Other commands
preserve machine-readable JSON output without a pseudo-terminal.
