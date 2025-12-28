## 1) Overview

**Problem:** install tools that are published on GitHub/GitLab/etc. (typically as release assets, raw files, or tarballs) but aren’t packaged for your distro. You want:

* Declarative “packages” stored locally (e.g. `/var/lib/ghpm/packages/<name>/package.yaml`)
* Reproducible installs (version pinned or version-resolved deterministically)
* Safe upgrades/removals with a receipt of installed files
* Works well for “drop a binary in `/usr/local/bin` + systemd unit + symlinks”

**Core idea:** each package has:

* a **manifest** (what to fetch + what to install)
* an **installed receipt** (exact version + exact file list + hashes) stored in ghpm state

---

## 2) Goals / Non-goals

### Goals

* Install/remove/upgrade packages described in local manifests.
* Support fetching from:

  * GitHub releases (assets + checksums)
  * GitLab releases (assets)
  * Generic HTTP URLs (“direct download”)
* Support installing:

  * a single asset file (binary)
  * tar.* extraction with filtering
  * raw files (e.g. systemd unit)
  * symlinks
* Track ownership of installed files and enable clean removal.
* Atomic-ish upgrades: either new version fully installed or old stays intact.
* Support `install-all` for everything under `/var/lib/ghpm/packages`.

### Non-goals

* Dependency resolution like apt/pacman.
* Rebuilding from source (no compilation pipeline).
* Managing libraries or system-wide ABI integration.
* Replacing `/usr` packaging; ghpm primarily targets `/usr/local`, `/etc`, `/opt`.

---

## 3) Terminology

* **Manifest**: `package.yaml` describing source + artifacts + install mapping.
* **Receipt**: state record for an installed package (version, resolved URLs, file list, hashes).
* **Artifact**: a thing to fetch/install: asset, url, tar extraction, local file, symlink.
* **Source kind**: github/gitlab/http/… determines how versions/assets are discovered.

---

## 4) Filesystem layout

### Package definitions

* `/var/lib/ghpm/packages/<name>/package.yaml`
* Optional package-local files:

  * `/var/lib/ghpm/packages/<name>/files/...` (templates, units, extra config)

### Runtime state & cache

* `/var/lib/ghpm/state/installed.json` (or `installed.db` later)
* `/var/lib/ghpm/state/receipts/<name>.json` (one file per pkg)
* `/var/cache/ghpm/downloads/` (download cache keyed by URL+etag or checksum)
* `/var/lib/ghpm/work/` (temporary staging during install/upgrade)
* `/var/lock/ghpm.lock` (global lock)

---

## 5) CLI specification

### Primary commands

* `ghpm list`
  List known packages (manifests found), show installed version if installed.

* `ghpm status [name]`
  Show installed receipt, files, and whether they still match hashes on disk.

* `ghpm install <name>|--all [--version <v>] [--force]`
  Resolve version (unless pinned/explicit), fetch, install, write receipt.

* `ghpm remove <name> [--purge]`
  Remove files tracked in receipt.

* `ghpm upgrade <name>|--all [--dry-run]`
  Resolve latest (or configured channel), compare to installed, upgrade if newer.

### Global flags

* `--root <path>`: for chroot installs (default `/`)
* `--packages-dir <path>`: default `/var/lib/ghpm/packages`
* `--state-dir <path>`: default `/var/lib/ghpm/state`
* `--cache-dir <path>`: default `/var/cache/ghpm`
* `--json`: machine-readable output

Exit codes: `0` success, `1` generic error, `2` invalid manifest, `3` fetch error, `4` install conflict, `5` verification failed.

---

## 6) Manifest format (package.yaml)

Use YAML with a stable schema version.

### Top-level structure

```yaml
name: k3s
description: Lightweight Kubernetes
source:
  kind: github
  repo: k3s-io/k3s
install:
  # ordered actions
  - type: asset
    name: k3s                      # exact asset name OR pattern
    pattern: 'k3s'                 # alternative to exact name
    mode: "0755"
    target: /usr/local/bin/k3s

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

### Action types

#### `asset` (release asset)

Fetch a GitHub/GitLab release asset and install it.

Fields:

* `name` OR `pattern` (glob/regex)
* `target` (absolute path)
* `mode` (octal string)

Templating allowed in `name/target`:

* `{version}`, `{tag}`, `{os}`, `{arch}`, `{repo}`, `{name}`

#### `url` (direct HTTP fetch)

Fields:

* `url`
* `target`
* `mode`

#### `file` (package-local file)

Fields:

* `path` (relative to package dir, e.g. `files/k3s.service`)
* `target`
* `mode`

#### `symlink`

Fields:

* `target` (absolute path of link)
* `to` (either absolute path or relative to link dir)

#### `extract` (tar/zip extraction)

Fetch an archive (asset/url/file) and extract a subset.

Fields:

* `from`:

  * `{type: asset, name/pattern: ...}` or `{type: url, url: ...}` or `{type: file, path: ...}`
* `format`: `tar.gz|tar.xz|zip|auto`
* `stripComponents`: integer
* `targetDir`: absolute dir (e.g. `/usr/local`)
* `pick`: list of globs inside archive (optional)
* `omit`: list of globs inside archive (optional)

Rules:

* If `pick` set, only those are extracted
* If `omit` set, everything but those are extracted
* Extraction occurs into a staging dir first; then files are copied into target.

#### `mkdir`

Ensure directory exists with mode/owner.
Fields: `path`, `mode`, `owner`, `group`

---

## 7) Version resolution rules

### GitHub

* Resolve “release list” via API.
* Ignore prereleases/drafts.
* Determine “highest” version:

  * Prefer semantic version parsing when tags resemble semver
  * Otherwise fall back to GitHub publish date.

### GitLab

* Similar: use GitLab releases endpoint.

### Determinism

Receipt stores:

* resolved tag/version string
* release ID (if any)
* exact asset URLs and digests (if available)

---

## 8) Install/upgrade algorithm (atomic and safe)

### Locking

* Take global lock for any mutating command (install/remove/upgrade).
* Optionally also take per-package lock to allow parallel operations later.

### Staging

For install/upgrade:

1. Parse manifest + validate schema.
2. Resolve version + expand templates.
3. Build an **install plan**:

   * list of fetches
   * list of filesystem operations
4. Fetch all remote artifacts to workdir/cache.
5. Apply filesystem changes in a **transaction-like flow**:

   * For each file target:

     * If target exists and is owned by another package → conflict unless `--force`
     * Write to temp file in same filesystem (`<target>.ghpm.new`)
     * `fsync`, then `rename()` over target (atomic replace)
   * For symlinks: create temp symlink then rename.
   * For directories: create if missing.
6. Write receipt only after all steps succeed.

### Upgrade

Upgrade is install with extra steps:

* Load existing receipt.
* Install new version (same transaction mechanics).
* On success, remove obsolete files that were owned by package previously but are not in the new receipt *unless marked preserved*.

### Rollback strategy (minimal but useful)

* If a step fails mid-transaction, ghpm should:

  * not write the new receipt
  * attempt to restore from backups for any targets already replaced:

    * keep `<target>.ghpm.bak` during operation
* Receipt update is last; so “installed version” remains consistent.

---

## 9) Receipt / state format

### `/var/lib/ghpm/state/installed.json`

```json
{
  "schema": 1,
  "installed": {
    "k3s": {
      "version": "v1.35.0+k3s1",
      "receipt": "receipts/k3s.json",
      "installedAt": "2025-12-27T14:10:00+01:00"
    }
  }
}
```

### `/var/lib/ghpm/state/receipts/k3s.json`

```json
{
  "schema": 1,
  "name": "k3s",
  "source": {
    "kind": "github",
    "repo": "k3s-io/k3s",
    "tag": "v1.35.0+k3s1",
    "releaseId": 123456789
  },
  "platform": { "os": "linux", "arch": "amd64" },
  "artifacts": [
    {
      "type": "asset",
      "name": "k3s",
      "url": "https://github.com/.../download/.../k3s",
      "sha256": "…",
      "size": 12345678
    }
  ],
  "files": [
    {
      "path": "/usr/local/bin/k3s",
      "type": "file",
      "mode": 493,
      "sha256": "…"
    },
    {
      "path": "/usr/local/bin/kubectl",
      "type": "symlink",
      "to": "k3s"
    }
  ]
}
```

Notes:

* Store `mode` as integer (493 = 0755) in receipt.
* Hash symlink targets too (store `to`).
* `sha256` for installed files is valuable for `verify`.

---

## 10) Ownership, conflicts, and safety rules

### Ownership model

A path is “owned” if it appears in a receipt’s `files`.

Conflicts:

* If installing would overwrite a file owned by *another* package → error.
* If file exists but is unowned → error.

### Removal rules

* Remove only owned paths.
* For directories, remove only if empty and owned (or created by ghpm and recorded).
* Provide `preserve: true` on a file action (typical for `/etc/...`), so `remove` leaves it unless `--purge`.

---

## 11) systemd integration (optional but practical)

Add `postInstall` actions (and `postRemove`), which is a list of shell commands to run.

Important: these actions should be best-effort and clearly reported (don’t partially install because `systemctl start` failed).

---

## 12) Security / integrity

Minimum viable:

* HTTPS only for remote fetches (unless `--allow-insecure`).
* Support SHA256 verification when upstream provides checksum file asset; ghpm can parse and match the asset name

---

## 13) Configuration

### `/etc/ghpm/config.yaml`

```yaml
packagesDir: /var/lib/ghpm/packages
stateDir: /var/lib/ghpm/state
cacheDir: /var/cache/ghpm
network:
  timeoutSeconds: 30
  retries: 2
```

---

## 14) Extensibility

### Source kinds

Implement as drivers:

* `github`: list releases, pick release, list assets
* `gitlab`: same
* `http`: no discovery; version must be pinned or derived
* future: `gitea`, `forgejo`, `sourcehut`, `bitbucket`

Driver interface (conceptually):

* `resolveVersion(manifest, platform) -> ResolvedVersion`
* `resolveAssets(resolvedVersion, manifest) -> list[Asset]`
* `fetch(asset/url) -> localPath + metadata`

### Action types

Also pluggable:

* `asset`, `url`, `file`, `extract`, `symlink`, `systemd`, …

---

## 15) Example: k3s manifest (adapted to the schema)

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

  - type: symlink
    target: /usr/local/bin/ctr
    to: k3s

  - type: symlink
    target: /usr/local/bin/crictl
    to: k3s

  - type: url
    url: https://raw.githubusercontent.com/k3s-io/k3s/refs/heads/main/k3s.service
    target: /etc/systemd/system/k3s.service
    mode: "0644"

postInstall:
  - systemctl daemon-reload
```

