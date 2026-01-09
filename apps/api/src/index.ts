// biome-ignore lint/performance/noNamespaceImport: Sentry SDK official pattern
import * as Sentry from "@sentry/cloudflare";
import { Hono } from "hono";
import { cors } from "hono/cors";
import { logger } from "hono/logger";
import { secureHeaders } from "hono/secure-headers";
import { authMiddleware } from "./middleware/auth";
import { rateLimitMiddleware } from "./middleware/rate-limit";
import authRoutes from "./routes/auth";
import healRoutes from "./routes/heal";
import healthRoutes from "./routes/health";
import { invitationRoutes, orgInvitationsRoutes } from "./routes/invitations";
import organizationMembersRoutes from "./routes/organization-members";
import organizationsRoutes from "./routes/organizations";
import parseRoutes from "./routes/parse";
import projectsRoutes from "./routes/projects";
import webhookRoutes from "./routes/webhooks";
import type { Env } from "./types/env";

const app = new Hono<{ Bindings: Env }>();

// Global middleware
app.use("*", logger());

// Security headers
app.use(
  "*",
  secureHeaders({
    contentSecurityPolicy: {
      defaultSrc: ["'self'"],
    },
    xContentTypeOptions: "nosniff",
    xFrameOptions: "DENY",
    referrerPolicy: "strict-origin-when-cross-origin",
  })
);

// CORS configuration - restrict to allowed origins
app.use(
  "*",
  cors({
    origin: (origin, c) => {
      // Allow requests with no origin (like mobile apps or curl)
      if (!origin) {
        return null;
      }

      // Allow localhost for development
      if (
        origin.startsWith("http://localhost:") ||
        origin.startsWith("http://127.0.0.1:")
      ) {
        return origin;
      }

      // Allow configured allowed origins
      const allowedOrigins = c.env.ALLOWED_ORIGINS?.split(",") ?? [];
      if (allowedOrigins.includes(origin)) {
        return origin;
      }

      // Deny all other origins
      return null;
    },
    allowMethods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"],
    allowHeaders: ["Content-Type", "Authorization"],
    exposeHeaders: [
      "Content-Length",
      "X-RateLimit-Limit",
      "X-RateLimit-Remaining",
      "X-RateLimit-Reset",
    ],
    maxAge: 86_400,
    credentials: true,
  })
);

// Public routes
app.get("/", (c) => c.text("detent api"));
app.route("/health", healthRoutes);

// Webhook routes (verified by signature, not API key)
app.route("/webhooks", webhookRoutes);

// Protected routes (require JWT auth + rate limiting)
const api = new Hono<{ Bindings: Env }>();
api.use("*", authMiddleware);
api.use("*", rateLimitMiddleware);
api.route("/auth", authRoutes);
api.route("/parse", parseRoutes);
api.route("/heal", healRoutes);
api.route("/projects", projectsRoutes);
api.route("/organization-members", organizationMembersRoutes);
api.route("/organizations", organizationsRoutes);
api.route("/invitations", invitationRoutes);
api.route("/orgs/:orgId/invitations", orgInvitationsRoutes);

app.route("/v1", api);

// Export type for potential RPC client
export type AppType = typeof app;

export default Sentry.withSentry(
  (env: Env) => ({
    dsn: env.SENTRY_DSN,
    tracesSampleRate: 1.0,
  }),
  app
);
