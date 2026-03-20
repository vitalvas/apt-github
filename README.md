# apt-github

APT transport method for installing `.deb` packages directly from GitHub Releases.

## Overview

`apt-github` is an APT transport plugin that allows you to use GitHub repositories as APT package sources. It fetches `.deb` packages from GitHub Releases, verifies tag signatures via the GitHub API, and signs APT repository metadata with a local GPG key.

Designed to work with [goreleaser](https://goreleaser.com/) projects that publish `.deb` packages as release assets.

## How It Works

```
GitHub Release (signed tag)
       |
       v
GitHub API (verify tag signature)
       |
       v
Generate APT metadata (Release, Packages)
       |
       v
Clearsign with local Ed25519 GPG key
       |
       v
APT verifies signature via signed-by keyring
```

1. On `apt update`, the transport fetches the latest GitHub release and verifies the tag signature via the GitHub API.
2. It parses `.deb` assets and goreleaser's `checksums.txt` to generate APT-compatible `Packages` and `Release` index files.
3. The `Release` file is clearsigned to produce `InRelease`, which APT verifies using the local public key.
4. On `apt install`, the `.deb` is downloaded directly from the GitHub release asset URL.

## Setup

Generate the GPG signing key (requires root):

```bash
sudo apt-github setup
```

This creates:
- Private key in `/etc/apt-github/gpg/`
- Public key at `/etc/apt/keyrings/apt-github.gpg`

## Usage

Add a GitHub repository as an APT source:

```bash
echo 'deb [signed-by=/etc/apt/keyrings/apt-github.gpg] github://OWNER/REPO stable main' \
  | sudo tee /etc/apt/sources.list.d/REPO.list
```

Then use standard APT commands:

```bash
sudo apt update
sudo apt install PACKAGE_NAME
```

### Example

```bash
echo 'deb [signed-by=/etc/apt/keyrings/apt-github.gpg] github://vitalvas/systemd-supervisord stable main' \
  | sudo tee /etc/apt/sources.list.d/systemd-supervisord.list

sudo apt update
sudo apt install systemd-supervisord
```

## Requirements

- Go 1.22+ (build)
- `gpg` (runtime, for signing)
- GitHub releases with `.deb` assets (goreleaser naming convention)
- goreleaser's `checksums.txt` in the release assets

### Supported `.deb` Naming Patterns

Both goreleaser naming conventions are supported:

- `{name}_{version}_{os}_{arch}.deb` (e.g., `myapp_1.0.0_linux_amd64.deb`)
- `{name}_{version}_{arch}.deb` (e.g., `myapp_1.0.0_amd64.deb`)

## Security

The trust chain:

1. The GitHub release tag must be signed (verified via GitHub API's `verification.verified` field).
2. If verification passes, the transport signs the generated APT metadata with a local Ed25519 GPG key.
3. APT verifies the `InRelease` signature using the public key specified in `signed-by`.

If the GitHub tag signature verification fails, the transport refuses to serve signed metadata.

