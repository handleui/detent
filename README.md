# detent

Run GitHub Actions locally. When they fail, Claude fixes them.

## Install

```bash
curl -fsSL https://detent.dev/install.sh | bash
```

Installs `dt` to `~/.local/bin`. Update with `dt update`.

## Requirements

- Docker
- Anthropic API key (for `heal` command)

## Usage

```bash
dt check       # run workflows locally, extract errors
dt heal        # auto-fix errors with Claude
dt workflows   # enable/disable jobs
dt config      # manage settings
```

## Setup

```bash
export ANTHROPIC_API_KEY=sk-...
# or
dt config set api-key sk-...
```

## Workflow

1. `dt check` — see CI errors locally
2. `dt heal` — let Claude fix them
3. `dt check` — verify fixes
4. Push

## Platforms

Linux (x64, arm64) · macOS (Intel, Apple Silicon) · Windows (x64)

## Links

[Releases](https://github.com/detent-dev/detent/releases) · [Issues](https://github.com/detent-dev/detent/issues)

## License

MIT
