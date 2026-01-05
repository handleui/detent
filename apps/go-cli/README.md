# Detent CLI - npm Distribution

Run GitHub Actions locally with intelligent error troubleshooting. Detent executes workflows, parses errors, categorizes them intelligently, and leverages Claude AI with parallel iterations to troubleshoot issues automatically.

## Installation

```bash
npm install -g @detent/go-cli
```

After installation, the `dt` command will be available globally.

## Usage

```bash
dt --help
dt --version
```

## How It Works

Detent streamlines your CI/CD development workflow:

1. **Local Execution**: Runs GitHub Actions workflows on your local machine
2. **Error Detection**: Automatically parses and extracts errors from workflow outputs
3. **Smart Categorization**: Intelligently groups and categorizes error types
4. **AI Troubleshooting**: Prompts Claude in parallel with multiple iterations to diagnose and suggest fixes
5. **Iterative Refinement**: Continues troubleshooting across multiple rounds until issues are resolved

### Binary Distribution

This npm package automatically downloads the appropriate pre-built Go binary for your platform during installation:

- Linux: amd64, arm64
- macOS: amd64 (Intel), arm64 (Apple Silicon)
- Windows: amd64

## Troubleshooting

If the installation fails, you can manually download the binary from:
https://github.com/handleui/detent/releases

## License

MIT
