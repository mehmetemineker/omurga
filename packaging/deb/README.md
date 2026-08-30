# Debian package

The same `.deb` package format is supported by Debian and Ubuntu. Omurga
currently produces packages for `amd64` and `arm64`; the latter is suitable
for the tested Raspberry Pi Debian installation.

Build both packages on a Debian-based system:

```bash
bash packaging/deb/build.sh 0.1.0
```

Build from PowerShell with Docker:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.23-bookworm bash packaging/deb/build.sh 0.1.0
```

The output files are written to `dist/`:

```text
dist/omurga_0.1.0_amd64.deb
dist/omurga_0.1.0_arm64.deb
```

Install a local package:

```bash
sudo apt install ./omurga_0.1.0_arm64.deb
```

The package installs only `/usr/bin/omurga` and documentation. It does not
change the host or start services automatically; use `omurga host init` when
host provisioning is explicitly wanted.
