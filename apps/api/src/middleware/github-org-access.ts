import { and, eq, isNull } from "drizzle-orm";
import type { NodePgDatabase } from "drizzle-orm/node-postgres";
import type { Context, Next } from "hono";
import { createDb } from "../db/client";
import type * as schema from "../db/schema";
import {
  type Organization,
  organizationMembers,
  organizations,
} from "../db/schema";
import { getVerifiedGitHubIdentity } from "../lib/github-identity";
import { verifyGitHubMembership } from "../lib/github-membership";
import type { Env } from "../types/env";
// Import auth middleware to ensure type extensions are merged
import "../middleware/auth";

// Role assigned based on GitHub membership + installer status
export type OrgAccessRole = "owner" | "admin" | "member";

interface GitHubIdentity {
  userId: string;
  username: string;
}

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
  githubIdentity: GitHubIdentity;
  role: OrgAccessRole;
}

// Extend Hono context to include orgAccess
declare module "hono" {
  interface ContextVariableMap {
    orgAccess: OrgAccessContext;
  }
}

// Determine initial role for new members based on GitHub state
const seedRoleFromGitHub = (
  org: Organization,
  githubIdentity: GitHubIdentity,
  githubRole: "admin" | "member"
): OrgAccessRole => {
  // If user is the installer, they get "owner" regardless of GitHub role
  if (org.installerGithubId === githubIdentity.userId) {
    return "owner";
  }
  if (githubRole === "admin") {
    return "admin";
  }
  return "member";
};

// Handle GitHub organization membership check and role determination
const resolveGitHubOrgRole = async (
  db: NodePgDatabase<typeof schema>,
  org: Organization,
  userId: string,
  githubIdentity: GitHubIdentity,
  env: Env
): Promise<{ role: OrgAccessRole } | { error: string; status: number }> => {
  // Check for existing Detent membership first
  const existingMember = await db.query.organizationMembers.findFirst({
    where: and(
      eq(organizationMembers.userId, userId),
      eq(organizationMembers.organizationId, org.id)
    ),
  });

  // Verify membership via GitHub API (access gate)
  const membership = await verifyGitHubMembership(
    githubIdentity.username,
    org.providerAccountLogin,
    org.providerInstallationId as string,
    env
  );

  if (!membership.isMember) {
    return {
      error: "You are not a member of this GitHub organization",
      status: 403,
    };
  }

  if (existingMember) {
    // Existing member: use DB role for authorization
    // Update GitHub identity if changed
    if (
      existingMember.providerUserId !== githubIdentity.userId ||
      existingMember.providerUsername !== githubIdentity.username
    ) {
      await db
        .update(organizationMembers)
        .set({
          providerUserId: githubIdentity.userId,
          providerUsername: githubIdentity.username,
          providerLinkedAt: new Date(),
          updatedAt: new Date(),
        })
        .where(eq(organizationMembers.id, existingMember.id));
    }
    return { role: existingMember.role };
  }

  // New member: seed role from GitHub, then create record
  const role = seedRoleFromGitHub(
    org,
    githubIdentity,
    membership.role ?? "member"
  );

  await db.insert(organizationMembers).values({
    id: crypto.randomUUID(),
    organizationId: org.id,
    userId,
    role,
    providerUserId: githubIdentity.userId,
    providerUsername: githubIdentity.username,
    providerLinkedAt: new Date(),
  });

  return { role };
};

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
      if (githubIdentity.userId !== org.providerAccountId) {
        return c.json(
          {
            error: "Access denied",
            message: "You are not the owner of this GitHub account",
          },
          403
        );
      }
      role = "owner";
    } else {
      // GitHub Organization: resolve role via DB or GitHub
      const result = await resolveGitHubOrgRole(
        db,
        org,
        auth.userId,
        githubIdentity,
        c.env
      );

      if ("error" in result) {
        return c.json({ error: "Access denied", message: result.error }, 403);
      }
      role = result.role;
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
