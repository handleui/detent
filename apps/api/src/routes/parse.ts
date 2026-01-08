import { Hono } from "hono";

const app = new Hono();

// Maximum log size to prevent DoS (10MB)
const MAX_LOG_SIZE = 10 * 1024 * 1024;

// Valid log formats
const VALID_FORMATS = ["github-actions", "act"] as const;
type LogFormat = (typeof VALID_FORMATS)[number];

const isValidFormat = (format: unknown): format is LogFormat => {
  return (
    typeof format === "string" && VALID_FORMATS.includes(format as LogFormat)
  );
};

interface ParseRequestBody {
  logs?: string;
  format?: string;
}

// POST /parse - Parse CI logs and extract errors
app.post("/", async (c) => {
  let body: ParseRequestBody;
  try {
    body = await c.req.json<ParseRequestBody>();
  } catch {
    return c.json({ error: "Invalid JSON body" }, 400);
  }

  // Validate logs field
  if (body.logs !== undefined) {
    if (typeof body.logs !== "string") {
      return c.json({ error: "logs must be a string" }, 400);
    }
    if (body.logs.length > MAX_LOG_SIZE) {
      return c.json(
        { error: `logs exceeds maximum size of ${MAX_LOG_SIZE} bytes` },
        400
      );
    }
  }

  // Validate format field
  const format = body.format ?? "github-actions";
  if (!isValidFormat(format)) {
    return c.json(
      { error: `Invalid format. Must be one of: ${VALID_FORMATS.join(", ")}` },
      400
    );
  }

  // Stub response - will be wired to parseService when ready
  return c.json({
    message: "parse endpoint stub",
    received: {
      hasLogs: Boolean(body.logs),
      format,
    },
    errors: [],
  });
});

export default app;
