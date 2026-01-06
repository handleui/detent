import { Hono } from "hono";
import { streamSSE } from "hono/streaming";

// TODO: import { healerService } from "../services/healer";

const app = new Hono();

// POST /heal - Run healing loop with streaming response
app.post("/", async (c) => {
  // TODO: Implement request validation
  // TODO: Wire to healerService
  // TODO: Stream Claude responses back to client
  // TODO: Handle tool calls and progress updates

  const body = await c.req.json();

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
