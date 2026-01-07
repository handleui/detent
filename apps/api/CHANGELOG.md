# @detent/api

## 0.1.0

### Minor Changes

- Add API scaffold with Hono + Cloudflare Workers

  - Add `/health` endpoint (public)
  - Add `/v1/parse` endpoint stub for log parsing (protected)
  - Add `/v1/heal` endpoint stub with SSE streaming (protected)
  - Add `X-API-Key` auth middleware (stub for WorkOS)
  - Add service wrappers for `@detent/parser` and `@detent/healing`
  - Add Drizzle schema placeholder for PlanetScale
