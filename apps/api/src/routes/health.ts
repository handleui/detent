import { Hono } from "hono";
import { createDb } from "../db/client";
import type { Env } from "../types/env";

const app = new Hono<{ Bindings: Env }>();

// Simple health check - always returns 200 if the API is running
// OpenStatus monitors this endpoint for uptime
app.get("/", (c) =>
  c.json({
    status: "operational",
    version: "0.1.0",
    timestamp: new Date().toISOString(),
  })
);

// Deep health check - verifies database connectivity
// Use this for more thorough monitoring
app.get("/deep", async (c) => {
  const checks: {
    database: "operational" | "degraded" | "down";
  } = {
    database: "down",
  };

  let overallStatus: "operational" | "degraded" | "down" = "operational";

  // Check database connectivity
  try {
    const { client } = await createDb(c.env);
    await client.query("SELECT 1");
    await client.end();
    checks.database = "operational";
  } catch {
    checks.database = "down";
    overallStatus = "down";
  }

  const statusCode = overallStatus === "operational" ? 200 : 503;

  return c.json(
    {
      status: overallStatus,
      version: "0.1.0",
      timestamp: new Date().toISOString(),
      checks,
    },
    statusCode
  );
});

export default app;
