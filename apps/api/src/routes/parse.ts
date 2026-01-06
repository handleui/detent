import { Hono } from "hono";

// TODO: import { parseService } from "../services/parser";

const app = new Hono();

// POST /parse - Parse CI logs and extract errors
app.post("/", async (c) => {
  // TODO: Implement request validation
  // TODO: Wire to parseService
  // TODO: Handle GitHub Actions log format
  // TODO: Handle Act log format

  const body = await c.req.json();

  // Stub response
  return c.json({
    message: "parse endpoint stub",
    received: {
      hasLogs: Boolean(body.logs),
      format: body.format ?? "github-actions",
    },
    errors: [],
  });
});

export default app;
