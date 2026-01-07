---
"@detent/cli": minor
---

Add auth commands (login, logout, status) using OAuth 2.0 Device Authorization flow.
Credentials are stored securely in .detent/credentials.json with automatic token refresh.

Add organization management commands (create, list, status, members, join, leave).
Add link commands for binding repositories to organizations (link, status, unlink).
Add whoami command for displaying current user identity with optional debug info.
Add centralized API client library with typed endpoints for organizations, auth, and user info.
