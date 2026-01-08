/**
 * Rate limiting middleware using Upstash Redis
 *
 * Uses sliding window algorithm to limit requests per user.
 * Must be applied AFTER authMiddleware (requires userId from context).
 * Fails open for availability - allows requests if Redis is unavailable.
 */

import { Ratelimit } from "@upstash/ratelimit";
import { Redis } from "@upstash/redis";
import type { Context, Next } from "hono";
import type { Env } from "../types/env";

// Ephemeral cache to reduce Redis calls (persists across invocations)
const cache = new Map();

// Cache Ratelimit instances per environment (keyed by Redis URL)
const ratelimitInstances = new Map<string, Ratelimit>();

const getRatelimit = (env: Env): Ratelimit => {
  const cacheKey = env.UPSTASH_REDIS_REST_URL;

  const existing = ratelimitInstances.get(cacheKey);
  if (existing) {
    return existing;
  }

  const redis = new Redis({
    url: env.UPSTASH_REDIS_REST_URL,
    token: env.UPSTASH_REDIS_REST_TOKEN,
  });

  const ratelimit = new Ratelimit({
    redis,
    limiter: Ratelimit.slidingWindow(100, "1 m"),
    prefix: "detent:ratelimit",
    ephemeralCache: cache,
    timeout: 2000,
    analytics: true,
  });

  ratelimitInstances.set(cacheKey, ratelimit);
  return ratelimit;
};

export const rateLimitMiddleware = async (
  c: Context<{ Bindings: Env }>,
  next: Next
): Promise<Response | undefined> => {
  const auth = c.get("auth");

  if (!auth?.userId) {
    console.error("Rate limit middleware: missing auth context");
    return c.json({ error: "Unauthorized" }, 401);
  }

  const identifier = auth.userId;

  try {
    const ratelimit = getRatelimit(c.env);
    const { success, limit, remaining, reset, reason } =
      await ratelimit.limit(identifier);

    c.header("X-RateLimit-Limit", limit.toString());
    c.header("X-RateLimit-Remaining", remaining.toString());
    c.header("X-RateLimit-Reset", reset.toString());

    // Log timeout events for monitoring
    if (reason === "timeout") {
      console.warn("Rate limit timeout - check Redis connectivity", {
        userId: identifier,
      });
    }

    if (!success) {
      return c.json(
        {
          error: "Rate limit exceeded",
          retryAfter: reset,
        },
        429
      );
    }
  } catch (error) {
    // Fail open for availability - allow request if Redis fails
    console.error("Rate limit check failed:", {
      error: error instanceof Error ? error.message : String(error),
      userId: identifier,
    });
  }

  await next();
  return undefined;
};
