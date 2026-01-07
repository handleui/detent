---
"@detent/api": minor
---

Add GitHub App integration with webhook handlers for installation lifecycle events.
Includes PostgreSQL database schema (teams, projects, team_members) via Drizzle ORM,
JWT authentication middleware for WorkOS AuthKit, and webhook signature verification.

Add organization management endpoints (status, sync with GitHub installation state).
Add project management routes with CRUD operations and repo synchronization.
Add organization member management (list members, verify GitHub org membership).
Add rate limiting middleware using Upstash Redis with IP-based and user-based limits.
Add input validation utilities and encryption helpers for sensitive data.
Add comprehensive test suites for auth, organizations, and webhooks.
