import { and, eq, isNull } from "drizzle-orm";
import type { Context, Next } from "hono";
import { createDb } from "../db/client";
import { organizations } from "../db/schema";
import { getVerifiedGitHubIdentity } from "../lib/github-identity";
import { verifyGitHubMembership } from "../lib/github-membership";
import type { Env } from "../types/env";
// Import auth middleware to ensure type extensions are merged
import "../middleware/auth";

// Role assigned based on GitHub membership + installer status
export type OrgAccessRole = "owner" | "admin" | "member";

export interface OrgAccessContext {
  organization: {
    id: string;
    slug: string;
    name: string;
    provider: "github" | "gitlab";
    providerAccountLogin: string;
    providerAccountType: "organization" | "user";
    providerInstallationId: string | null;
    installerGithubId: string | null;
  };
  githubIdentity: {
    userId: string;
    username: string;
  };
  role: OrgAccessRole;
}

// Extend Hono context to include orgAccess
declare module "hono" {
  interface ContextVariableMap {
    orgAccess: OrgAccessContext;
  }
}

// Middleware that verifies GitHub org membership on-demand
// Sets "orgAccess" in context with verified role
export const githubOrgAccessMiddleware = async (
  c: Context<{ Bindings: Env }>,
  next: Next
): Promise<Response | undefined> => {
  const auth = c.get("auth");
  if (!auth?.userId) {
    return c.json({ error: "Authentication required" }, 401);
  }

  // Get org identifier from path params
  const orgIdOrSlug = c.req.param("orgId") || c.req.param("organizationId");
  if (!orgIdOrSlug) {
    return c.json({ error: "Organization identifier required" }, 400);
  }

  // Fetch organization
  const { db, client } = await createDb(c.env);
  try {
    const org = await db.query.organizations.findFirst({
      where: and(
        // Match by ID or slug
        orgIdOrSlug.includes("/")
          ? eq(organizations.slug, orgIdOrSlug)
          : eq(organizations.id, orgIdOrSlug),
        isNull(organizations.deletedAt)
      ),
    });

    if (!org) {
      return c.json({ error: "Organization not found" }, 404);
    }

    if (org.suspendedAt) {
      return c.json({ error: "Organization is suspended" }, 403);
    }

    // Only GitHub orgs supported for now (GitLab uses different auth)
    if (org.provider !== "github") {
      return c.json(
        { error: "GitLab organizations use token-based access" },
        400
      );
    }

    if (!org.providerInstallationId) {
      return c.json(
        { error: "GitHub App not installed for this organization" },
        400
      );
    }

    // Get user's verified GitHub identity from WorkOS
    const githubIdentity = await getVerifiedGitHubIdentity(
      auth.userId,
      c.env.WORKOS_API_KEY
    );

    if (!githubIdentity) {
      return c.json(
        {
          error: "GitHub account not linked",
          code: "GITHUB_NOT_LINKED",
          message:
            "Please link your GitHub account via WorkOS to access organizations",
        },
        403
      );
    }

    // Determine role based on org type
    let role: OrgAccessRole;

    if (org.providerAccountType === "user") {
      // Personal GitHub account: only the owner can access
      // Check if user's GitHub ID matches the account owner
      if (githubIdentity.userId === org.providerAccountId) {
        role = "owner";
      } else {
        return c.json(
          {
            error: "Access denied",
            message: "You are not the owner of this GitHub account",
          },
          403
        );
      }
    } else {
      // GitHub Organization: verify membership via API
      const membership = await verifyGitHubMembership(
        githubIdentity.username,
        org.providerAccountLogin,
        org.providerInstallationId,
        c.env
      );

      if (!membership.isMember) {
        return c.json(
          {
            error: "Access denied",
            message: "You are not a member of this GitHub organization",
          },
          403
        );
      }

      // Map GitHub role to Detent role
      // If user is the installer, they get "owner" regardless of GitHub role
      if (org.installerGithubId === githubIdentity.userId) {
        role = "owner";
      } else if (membership.role === "admin") {
        role = "admin";
      } else {
        role = "member";
      }
    }

    // Set context for downstream handlers
    const orgAccess: OrgAccessContext = {
      organization: {
        id: org.id,
        slug: org.slug,
        name: org.name,
        provider: org.provider,
        providerAccountLogin: org.providerAccountLogin,
        providerAccountType: org.providerAccountType,
        providerInstallationId: org.providerInstallationId,
        installerGithubId: org.installerGithubId,
      },
      githubIdentity,
      role,
    };

    c.set("orgAccess", orgAccess);

    console.log(
      `[org-access] ${githubIdentity.username} has ${role} access to ${org.slug}`
    );

    await next();
    return undefined;
  } finally {
    await client.end();
  }
};

// Helper to require specific roles
export const requireRole =
  (...allowedRoles: OrgAccessRole[]) =>
  async (
    c: Context<{ Bindings: Env }>,
    next: Next
  ): Promise<Response | undefined> => {
    const orgAccess = c.get("orgAccess");
    if (!orgAccess) {
      return c.json({ error: "Organization access not verified" }, 500);
    }

    if (!allowedRoles.includes(orgAccess.role)) {
      return c.json(
        {
          error: "Insufficient permissions",
          required: allowedRoles,
          current: orgAccess.role,
        },
        403
      );
    }

    await next();
    return undefined;
  };
