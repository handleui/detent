# Detent

Self-healing CI/CD that runs on PR's, it is very much NOT ready, please, avoid installing while I fix my mess

## Install

```bash
curl -fsSL https://detent.sh/install.sh | bash
```

Installs `dt` to `~/.local/bin`. Update with `dt update`.

## Requirements
BYOK for convenience, more providers on the way

- Anthropic API key (for `heal` command)

## Usage

```bash
dt mock        # run workflows locally with act, it's a BETA, act is very choppy and needs a lot more attention
dt config      # manage settings
dt init        # gets your api key for AI healing, probably about to be deprecated since we're moving to the cloud
```

## Platforms

Linux (x64, arm64) · macOS (Intel, Apple Silicon) · Windows (x64)

## Links

[Github Releases](https://github.com/handleui/detent/releases) · [Issues](https://github.com/handleui/detent/issues)

## License

MIT
