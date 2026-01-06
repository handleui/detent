import type { Context, Next } from "hono";
import { HTTPException } from "hono/http-exception";

// TODO: Replace with WorkOS AuthKit
// TODO: Add organization context
// TODO: Add rate limiting per API key

interface AuthContext {
  apiKey: string;
  // TODO: Add organization ID, user ID, permissions
}

declare module "hono" {
  interface ContextVariableMap {
    auth: AuthContext;
  }
}

export const authMiddleware = async (c: Context, next: Next) => {
  const apiKey = c.req.header("X-API-Key");

  if (!apiKey) {
    throw new HTTPException(401, { message: "Missing X-API-Key header" });
  }

  // TODO: Validate API key against WorkOS/database
  // TODO: Check rate limits
  // TODO: Load organization context

  // Stub: Accept any non-empty API key
  c.set("auth", { apiKey });

  await next();
};
