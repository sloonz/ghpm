# Manifest Reference

Manifests live at `/var/lib/ghpm/packages/<name>/package.yaml` and define
how to fetch and install a tool.

## Minimal example

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
```

## Top-level fields

- `name` (string, required): Package name.
- `description` (string, optional): Short description.
- `source` (object, optional): Where releases/assets come from.
- `install` (list, required): Ordered list of install actions.
- `postInstall` (list, optional): Shell commands to run after install.
- `postRemove` (list, optional): Shell commands to run after remove.

## Source

```yaml
source:
  kind: github|gitlab|http
  repo: owner/name   # github/gitlab only
```

Notes:
- `http` does not support discovery, so `--version` is required for install/upgrade.
- `repo` is `owner/name` for GitHub/GitLab.

## Templates

In `name`, `pattern`, `target`, and `url` fields you can use:

```
{version} {tag} {os} {arch} {repo} {name}
```

## Install actions

Actions run in order. All `target` paths are absolute.

### `asset`

Fetch a GitHub/GitLab release asset and install it.

```yaml
- type: asset
  name: k3s           # exact asset name, or use pattern
  pattern: 'k3s'      # regex fallback if name not set
  target: /usr/local/bin/k3s
  mode: "0755"
  preserve: false
```

### `url`

Fetch a direct URL and install it.

```yaml
- type: url
  url: https://example.com/k3s.service
  target: /etc/systemd/system/k3s.service
  mode: "0644"
  preserve: true
```

### `file`

Install a local file from the package directory.

```yaml
- type: file
  path: files/k3s.service
  target: /etc/systemd/system/k3s.service
  mode: "0644"
  preserve: true
```

### `symlink`

Create a symlink.

```yaml
- type: symlink
  target: /usr/local/bin/kubectl
  to: k3s            # relative to link dir or absolute
```

### `extract`

Extract an archive into a target directory.

```yaml
- type: extract
  from:
    type: asset
    name: k3s.tar.gz
  format: tar.gz     # tar.gz|tar.xz|zip|auto
  stripComponents: 1
  targetDir: /usr/local
  pick:
    - bin/k3s
```

`from` can be:

```yaml
from: { type: asset, name: "...", pattern: "..." }
from: { type: url, url: "..." }
from: { type: file, path: "files/archive.tar.gz" }
```

### `mkdir`

Ensure a directory exists.

```yaml
- type: mkdir
  path: /etc/k3s
  mode: "0755"
```

## Preserve

Set `preserve: true` on file actions to keep them on `remove` unless `--purge`
is used.

## Example with systemd

```yaml
name: k3s
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
    preserve: true
postInstall:
  - systemctl daemon-reload
```
