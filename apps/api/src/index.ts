import { Hono } from "hono";
import { cors } from "hono/cors";
import { logger } from "hono/logger";
import { authMiddleware } from "./middleware/auth";
import githubLinkRoutes from "./routes/github-link";
import healRoutes from "./routes/heal";
import healthRoutes from "./routes/health";
import parseRoutes from "./routes/parse";
import webhookRoutes from "./routes/webhooks";
import type { Env } from "./types/env";

const app = new Hono<{ Bindings: Env }>();

// Global middleware
app.use("*", logger());
app.use("*", cors());

// Public routes
app.get("/", (c) => c.text("detent api"));
app.route("/health", healthRoutes);

// Webhook routes (verified by signature, not API key)
app.route("/webhooks", webhookRoutes);

// Protected routes (require JWT auth)
const api = new Hono<{ Bindings: Env }>();
api.use("*", authMiddleware);
api.route("/parse", parseRoutes);
api.route("/heal", healRoutes);
api.route("/github", githubLinkRoutes);

app.route("/v1", api);

// Export type for potential RPC client
export type AppType = typeof app;

export default app;
