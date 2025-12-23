# Detent CLI - npm Distribution

This package provides npm distribution for the Detent CLI, a task management and time tracking tool written in Go.

## Installation

```bash
npm install -g @detent/cli
```

After installation, the `dt` command will be available globally.

## Usage

```bash
dt --help
dt --version
```

## How It Works

This package automatically downloads the appropriate pre-built binary for your platform (Linux, macOS, or Windows) from GitHub releases during installation.

Supported platforms:

- Linux: amd64, arm64
- macOS: amd64 (Intel), arm64 (Apple Silicon)
- Windows: amd64

## Troubleshooting

If the installation fails, you can manually download the binary from:
https://github.com/handleui/detent/releases

## License

MIT
