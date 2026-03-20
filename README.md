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

## Installation

Download and install the latest `.deb` package from [GitHub Releases](https://github.com/vitalvas/apt-github/releases).

The postinstall script automatically generates the GPG signing key. To regenerate it manually:

```bash
sudo /usr/lib/apt/methods/github setup
```

The signing key is stored at:
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

Once installed, apt-github can manage its own updates:

```bash
echo 'deb [signed-by=/etc/apt/keyrings/apt-github.gpg] github://vitalvas/apt-github stable main' \
  | sudo tee /etc/apt/sources.list.d/apt-github.list

sudo apt update
sudo apt install apt-github
```

### DEB822 Format

You can also use the modern DEB822 format (`.sources` files):

```bash
cat <<EOF | sudo tee /etc/apt/sources.list.d/apt-github.sources
Types: deb
URIs: github://vitalvas/apt-github
Suites: stable
Components: main
Signed-By: /etc/apt/keyrings/apt-github.gpg
EOF
```

### Version History

By default, the last 3 releases are available for version pinning. To change the limit, add the `versions` query parameter:

```
deb [signed-by=/etc/apt/keyrings/apt-github.gpg] github://OWNER/REPO?versions=20 stable main
```

> [!WARNING]
> Each version requires downloading the `.deb` file to extract package metadata during `apt update` (results are cached on disk for subsequent runs). Higher version counts increase the initial `apt update` time and GitHub API usage. The unauthenticated GitHub API rate limit is 60 requests per hour.

### Priority Pinning

APT priority pinning lets you control how packages from GitHub repos are preferred relative to other sources. The transport generates `Origin: github.com` and `Label: {owner}/{repo}` in the Release file.

Pin a specific repo higher than default:

```
# /etc/apt/preferences.d/apt-github.pref
Package: *
Pin: release o=github.com,l=vitalvas/apt-github
Pin-Priority: 990
```

Pin all GitHub repos lower than official:

```
# /etc/apt/preferences.d/apt-github.pref
Package: *
Pin: release o=github.com
Pin-Priority: 400
```

Verify with:

```bash
apt-cache policy <package-name>
```

### Cache

Release metadata and package control data are cached locally at `/var/cache/apt-github/` in a tree organized by `{owner}/{repo}/{tag}/`. The release metadata cache has a 5-minute TTL; control metadata and downloaded `.deb` files are cached indefinitely. Stale tag directories are automatically removed when releases are refreshed.

To clear the cache:

```bash
sudo /usr/lib/apt/methods/github clean
```

### Authentication

To avoid GitHub API rate limits (60 requests/hour unauthenticated), provide a Personal Access Token (PAT). The token is read from the following sources in order:

1. Environment variable `GITHUB_TOKEN`
2. File `/etc/apt-github/token`

To set up a token:

```bash
echo "ghp_yourtoken" | sudo tee /etc/apt-github/token
sudo chmod 600 /etc/apt-github/token
```

A token with no scopes (public repo access only) is sufficient.

## Requirements

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

