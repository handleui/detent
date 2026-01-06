---
"@detent/healing": minor
---

Initial release of the Detent healing module (AI-powered error correction)

### Features

- **Anthropic Claude Integration**: Direct API client for Claude models
- **Agentic Error Fixing Loop**: Autonomous iteration over errors until resolution
- **File System Tools**: Safe read/write/edit operations for code modifications
- **Structured Prompt System**: Modular prompt components for consistent AI behavior
- **Evaluation Framework**: Braintrust integration for measuring fix quality

### Architecture

- Utility-first design: tools and prompts are composable and testable
- Streaming support for real-time feedback during fixes
- Configurable model selection (Claude Sonnet, Haiku, etc.)
- Sandboxed file operations to prevent unintended changes

### Technical Details

- Built on `@anthropic-ai/sdk` for type-safe API access
- `fast-glob` for efficient file discovery
- Comprehensive TypeScript types for tool inputs/outputs
