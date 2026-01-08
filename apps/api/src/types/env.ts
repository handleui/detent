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

  // WorkOS User Management credentials
  WORKOS_CLIENT_ID: string;
  WORKOS_API_KEY: string; // For fetching user details and identities

  // OAuth configuration (optional)
  ALLOWED_REDIRECT_URIS?: string; // Comma-separated list of allowed redirect URIs

  // CORS configuration (optional)
  ALLOWED_ORIGINS?: string; // Comma-separated list of allowed CORS origins

  // Encryption key for sensitive data (GitLab tokens, etc.)
  // Generate with: openssl rand -base64 32
  ENCRYPTION_KEY?: string;

  // Upstash Redis for rate limiting
  UPSTASH_REDIS_REST_URL: string;
  UPSTASH_REDIS_REST_TOKEN: string;

  // OpenStatus monitoring (optional)
  // Your OpenStatus status page slug for public status checks
  OPENSTATUS_SLUG?: string;
}
