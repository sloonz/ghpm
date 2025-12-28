# ghpm

GitHub/GitLab package manager for installing release assets and files into
`/usr/local`, `/etc`, or `/opt` based on local manifests.

This project was implemented by GPT-5.2-Codex.

## Build

```bash
go build ./cmd/ghpm
```

## Configuration

Optional config at `/etc/ghpm/config.yaml`:

```yaml
packagesDir: /var/lib/ghpm/packages
stateDir: /var/lib/ghpm/state
cacheDir: /var/cache/ghpm
network:
  timeoutSeconds: 30
  retries: 2
```

Global flags can override these:

```
--root --packages-dir --state-dir --cache-dir --json --config --silent --verbose
```

## Commands

```bash
ghpm list
ghpm status <name>
ghpm install <name> [--version <v>] [--force]
ghpm install --all
ghpm remove <name> [--purge]
ghpm upgrade <name>
ghpm upgrade --all [--dry-run]
```

## Manifest format

Example `package.yaml`:

```yaml
name: k3s
description: Lightweight Kubernetes
source:
  kind: github
  repo: k3s-io/k3s
install:
  - type: asset
    name: k3s
    target: /usr/local/bin/k3s
    mode: "0755"
  - type: symlink
    target: /usr/local/bin/kubectl
    to: k3s
  - type: url
    url: https://raw.githubusercontent.com/k3s-io/k3s/refs/heads/main/k3s.service
    target: /etc/systemd/system/k3s.service
    mode: "0644"
postInstall:
  - systemctl daemon-reload
```

Supported `install` action types:

- `asset`: fetch release asset from GitHub/GitLab and install to target.
- `url`: fetch direct URL and install to target.
- `file`: install a package-local file from `files/`.
- `symlink`: create a symlink.
- `extract`: extract an archive (tar.gz, tar.xz, zip) into a target dir.
- `mkdir`: ensure a directory exists.

Template variables available in `name/target/url`:

```
{version} {tag} {os} {arch} {repo} {name}
```

## Filesystem layout

```
/var/lib/ghpm/packages/<name>/package.yaml
/var/lib/ghpm/state/installed.json
/var/lib/ghpm/state/receipts/<name>.json
/var/cache/ghpm/downloads/
```
