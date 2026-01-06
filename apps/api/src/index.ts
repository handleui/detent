import { Hono } from "hono";
import { cors } from "hono/cors";
import { logger } from "hono/logger";
import { authMiddleware } from "./middleware/auth";
import healRoutes from "./routes/heal";
import healthRoutes from "./routes/health";
import parseRoutes from "./routes/parse";

const app = new Hono();

// Global middleware
app.use("*", logger());
app.use("*", cors());

// Public routes
app.get("/", (c) => c.text("detent api"));
app.route("/health", healthRoutes);

// Protected routes (require X-API-Key)
const api = new Hono();
api.use("*", authMiddleware);
api.route("/parse", parseRoutes);
api.route("/heal", healRoutes);

app.route("/v1", api);

// Export type for potential RPC client
export type AppType = typeof app;

export default app;
