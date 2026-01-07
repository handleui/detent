import type { Hyperdrive } from "@cloudflare/workers-types";

// Cloudflare Worker environment bindings
// Set these via: npx wrangler secret put <NAME>

export interface Env {
  // GitHub App credentials
  GITHUB_APP_ID: string;
  GITHUB_CLIENT_ID: string;
  GITHUB_APP_PRIVATE_KEY: string;
  GITHUB_WEBHOOK_SECRET: string;

  // Database connection via Cloudflare Hyperdrive
  HYPERDRIVE: Hyperdrive;
  // Fallback for local dev / migrations
  DATABASE_URL?: string;

  // WorkOS AuthKit credentials
  WORKOS_CLIENT_ID: string;
  WORKOS_SUBDOMAIN: string;
}
