/**
 * JWT authentication middleware
 *
 * Validates WorkOS AuthKit access tokens from the Authorization header.
 * Sets userId and organizationId in context for downstream handlers.
 */

import type { Context, Next } from "hono";
import { verifyAccessToken } from "../lib/auth";
import type { Env } from "../types/env";

interface AuthContext {
  userId: string;
  organizationId?: string;
}

declare module "hono" {
  interface ContextVariableMap {
    auth: AuthContext;
  }
}

const BEARER_TOKEN_REGEX = /^Bearer\s+(.+)$/i;

const extractBearerToken = (header: string | undefined): string | null => {
  if (!header) {
    return null;
  }
  const match = header.match(BEARER_TOKEN_REGEX);
  return match?.[1] ?? null;
};

export const authMiddleware = async (
  c: Context<{ Bindings: Env }>,
  next: Next
): Promise<Response | undefined> => {
  const token = extractBearerToken(c.req.header("authorization"));

  if (!token) {
    return c.json({ error: "Missing authorization header" }, 401);
  }

  try {
    const payload = await verifyAccessToken(token, {
      clientId: c.env.WORKOS_CLIENT_ID,
    });

    c.set("auth", {
      userId: payload.sub,
      organizationId: payload.org_id,
    });

    await next();
    return undefined;
  } catch (error) {
    // Log verification failure details for debugging
    console.error("Token verification failed:", {
      error: error instanceof Error ? error.message : String(error),
      clientId: c.env.WORKOS_CLIENT_ID,
    });
    return c.json({ error: "Invalid or expired token" }, 401);
  }
};
