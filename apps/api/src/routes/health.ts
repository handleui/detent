import { Hono } from "hono";
import pkg from "../../package.json";
import { createDb } from "../db/client";
import type { Env } from "../types/env";

const app = new Hono<{ Bindings: Env }>();

// Health check - verifies API and database connectivity
// OpenStatus monitors this endpoint for uptime
app.get("/", async (c) => {
  const checks: {
    database: "operational" | "down";
  } = {
    database: "down",
  };

  let status: "operational" | "down" = "operational";

  try {
    const { client } = await createDb(c.env);
    await client.query("SELECT 1");
    await client.end();
    checks.database = "operational";
  } catch {
    checks.database = "down";
    status = "down";
  }

  return c.json(
    {
      status,
      version: pkg.version,
      timestamp: new Date().toISOString(),
      checks,
    },
    status === "operational" ? 200 : 503
  );
});

export default app;
