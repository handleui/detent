---
"@detent/cli": minor
---

Add heal command infrastructure, frankenstein command, and config management

- New `heal` command with Claude Agent SDK integration for AI-powered error fixing
- New `frankenstein` command for parallel Claude iterations
- New `config` command with interactive TUI for managing settings
- Agent tools: edit_file, glob, grep, read_file, run_command with security controls
- Heal loop with budget tracking and pricing calculations
- Trust prompts, API key prompts, and command approval workflows
- JSON config schema with IDE autocomplete (`https://detent.sh/detent.schema.json`)
- Install script hosted at `https://detent.sh/install.sh`
- Git worktree and commit improvements for heal operations
