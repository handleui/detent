import { Hono } from "hono";
import { streamSSE } from "hono/streaming";

const app = new Hono();

// Maximum number of errors to process
const MAX_ERRORS = 100;

// Maximum error message length (to prevent DoS)
const MAX_ERROR_LENGTH = 10_000;

interface ErrorItem {
  message?: string;
  file?: string;
  line?: number;
  column?: number;
  type?: string;
}

interface HealRequestBody {
  errors?: ErrorItem[];
  repository?: string;
  branch?: string;
}

const isValidError = (error: unknown): error is ErrorItem => {
  if (typeof error !== "object" || error === null) {
    return false;
  }
  const e = error as Record<string, unknown>;

  // Check optional string fields
  if (e.message !== undefined && typeof e.message !== "string") {
    return false;
  }
  if (e.file !== undefined && typeof e.file !== "string") {
    return false;
  }
  if (e.type !== undefined && typeof e.type !== "string") {
    return false;
  }

  // Check optional number fields
  if (e.line !== undefined && typeof e.line !== "number") {
    return false;
  }
  if (e.column !== undefined && typeof e.column !== "number") {
    return false;
  }

  // Validate message length if present
  if (typeof e.message === "string" && e.message.length > MAX_ERROR_LENGTH) {
    return false;
  }

  return true;
};

// POST /heal - Run healing loop with streaming response
app.post("/", async (c) => {
  let body: HealRequestBody;
  try {
    body = await c.req.json<HealRequestBody>();
  } catch {
    return c.json({ error: "Invalid JSON body" }, 400);
  }

  // Validate errors array if present
  if (body.errors !== undefined) {
    if (!Array.isArray(body.errors)) {
      return c.json({ error: "errors must be an array" }, 400);
    }

    if (body.errors.length > MAX_ERRORS) {
      return c.json(
        { error: `Too many errors. Maximum is ${MAX_ERRORS}` },
        400
      );
    }

    for (const error of body.errors) {
      if (!isValidError(error)) {
        return c.json({ error: "Invalid error object in errors array" }, 400);
      }
    }
  }

  // Validate optional string fields
  if (body.repository !== undefined && typeof body.repository !== "string") {
    return c.json({ error: "repository must be a string" }, 400);
  }
  if (body.branch !== undefined && typeof body.branch !== "string") {
    return c.json({ error: "branch must be a string" }, 400);
  }

  // Stub response - will stream from healerService when ready
  return streamSSE(c, async (stream) => {
    // Stub: Send initial event
    await stream.writeSSE({
      event: "status",
      data: JSON.stringify({ phase: "starting", message: "Healing loop stub" }),
    });

    // Stub: Acknowledge received errors
    await stream.writeSSE({
      event: "status",
      data: JSON.stringify({
        phase: "received",
        errorCount: body.errors?.length ?? 0,
      }),
    });

    // Stub: Complete
    await stream.writeSSE({
      event: "complete",
      data: JSON.stringify({
        success: true,
        patches: [],
        message: "heal endpoint stub - no actual healing performed",
      }),
    });
  });
});

export default app;
