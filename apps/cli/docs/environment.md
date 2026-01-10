# Environment Variables

The CLI handles environment variables differently in development vs production.

## Development

Create a `.env` file in `apps/cli/`:

```bash
# Required - get from WorkOS dashboard
WORKOS_CLIENT_ID=client_xxx

# Optional - point to local services
DETENT_API_URL=http://localhost:8787
DETENT_AUTH_URL=http://localhost:3001
```

The CLI loads this file automatically via `dotenv` when running in dev mode.

## Production Binaries

**Users don't need to set anything.** All values are baked into the binary at compile time.

When building binaries (`bun run build:binaries`), the build script:

| Variable | Required? | Default |
|----------|-----------|---------|
| `WORKOS_CLIENT_ID` | Yes | - |
| `DETENT_API_URL` | No | `https://api.detent.sh` |
| `DETENT_AUTH_URL` | No | `https://app.detent.sh` |

Example build command:

```bash
WORKOS_CLIENT_ID=client_xxx bun run build:binaries
```

The magic happens in `scripts/build-binaries.ts` - it uses Bun's `--define` flag to replace `process.env.X` references with literal strings at compile time. No runtime env lookup needed.

## How It Works

```typescript
// src/index.ts
declare const DETENT_PRODUCTION: boolean | undefined;

if (typeof DETENT_PRODUCTION === "undefined") {
  // Dev mode: load .env file
  const { config } = await import("dotenv");
  config({ path: ".env" });
}
// Production: DETENT_PRODUCTION is true, .env is never loaded
// All process.env.X calls are already replaced with actual values
```

## CI/CD

Set `WORKOS_CLIENT_ID` as a GitHub Actions secret. The other URLs use sensible defaults.
