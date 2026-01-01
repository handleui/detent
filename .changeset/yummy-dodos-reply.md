---
"@detent/cli": minor
---

Improved check command to support depends flags on yaml files, comprehensive safelist for production releases (are skipped)

- Check command properly creates a manifest of all jobs and steps
- We properly track the progress of all jobs and steps
- We inject bypasses for dependent jobs so we grep all errors
- Sensitive runs are skipped (production deployments, version bumps, npm releases, docker publishes, etc) and properly disclosed on the TUI
