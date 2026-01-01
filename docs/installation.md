# Installation

Install detent with a single command:

```bash
curl -fsSL https://detent.dev/install.sh | bash
```

This installs the `dt` binary to `~/.local/bin`.

## Options

### Custom install directory

```bash
curl -fsSL https://detent.dev/install.sh | DETENT_INSTALL_DIR=/usr/local/bin bash
```

### Install specific version

```bash
curl -fsSL https://detent.dev/install.sh | DETENT_VERSION=v1.2.3 bash
```

### Combine options

```bash
curl -fsSL https://detent.dev/install.sh | DETENT_VERSION=v1.0.0 DETENT_INSTALL_DIR=./bin bash
```

## Supported platforms

| OS      | Architecture |
|---------|--------------|
| Linux   | x64, arm64   |
| macOS   | x64, arm64   |
| Windows | x64          |

Apple Silicon Macs running under Rosetta 2 are automatically detected and will receive the native arm64 binary.

## Manual installation

Download binaries directly from [GitHub Releases](https://github.com/detent/cli/releases).

## Uninstall

Remove the binary:

```bash
rm ~/.local/bin/dt
```
