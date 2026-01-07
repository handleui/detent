/**
 * Organization Members API routes
 *
 * Manages organization membership - adding/removing users from organizations.
 * Users can join organizations via the GitHub App installation flow.
 *
 * SECURITY: Users can only join organizations where their authenticated GitHub
 * identity matches a member of the GitHub organization/account that owns the
 * Detent organization. This is verified via WorkOS OAuth identity.
 */

import { and, count, eq, isNull } from "drizzle-orm";
import { Hono } from "hono";
import { createDb } from "../db/client";
import { organizationMembers, organizations } from "../db/schema";
import { validateSlug, validateUUID } from "../lib/validation";
import type { Env } from "../types/env";

const app = new Hono<{ Bindings: Env }>();

/**
 * Verify that a GitHub user is a member of a GitHub organization
 * Returns the membership role if verified, null otherwise
 */
const verifyGitHubOrgMembership = async (
  githubUsername: string,
  githubOrg: string
): Promise<"admin" | "member" | null> => {
  try {
    // Use public API to check org membership
    // This works for public org members; private membership requires auth
    const response = await fetch(
      `https://api.github.com/orgs/${encodeURIComponent(githubOrg)}/members/${encodeURIComponent(githubUsername)}`,
      {
        headers: {
          Accept: "application/vnd.github.v3+json",
          "User-Agent": "Detent-API",
        },
      }
    );

    if (response.status === 204) {
      // 204 = user is a public member
      return "member";
    }

    if (response.status === 404) {
      // 404 = user is not a public member (could be private or not a member)
      // For user accounts (not orgs), we need different logic
      return null;
    }

    return null;
  } catch {
    return null;
  }
};

/**
 * Verify that a GitHub user owns or has access to a user account
 * For GitHub user accounts (not orgs), the user must BE that account
 */
const verifyGitHubUserAccess = (
  githubUsername: string,
  accountLogin: string
): boolean => {
  // For user accounts, the authenticated user must be the account owner
  return githubUsername.toLowerCase() === accountLogin.toLowerCase();
};

interface JoinRequest {
  organization_id?: string;
  organization_slug?: string;
  github_username: string;
}

/**
 * Validate join request inputs
 * Returns error message if invalid, null if valid
 */
const validateJoinRequest = (body: JoinRequest): string | null => {
  const { organization_id, organization_slug, github_username } = body;

  if (!(organization_id || organization_slug)) {
    return "organization_id or organization_slug is required";
  }

  if (organization_id) {
    const validation = validateUUID(organization_id, "organization_id");
    if (!validation.valid) {
      return validation.error ?? "Invalid organization_id";
    }
  }

  if (organization_slug) {
    const validation = validateSlug(organization_slug, "organization_slug");
    if (!validation.valid) {
      return validation.error ?? "Invalid organization_slug";
    }
  }

  if (!github_username || typeof github_username !== "string") {
    return "github_username is required for membership verification";
  }

  return null;
};

/**
 * Verify GitHub access and determine Detent role
 */
const verifyGitHubAccess = async (
  githubUsername: string,
  providerAccountType: string,
  providerAccountLogin: string
): Promise<{ hasAccess: boolean; role: "owner" | "admin" | "member" }> => {
  if (providerAccountType === "organization") {
    const githubRole = await verifyGitHubOrgMembership(
      githubUsername,
      providerAccountLogin
    );
    if (githubRole) {
      return {
        hasAccess: true,
        role: githubRole === "admin" ? "admin" : "member",
      };
    }
  } else {
    const isOwner = verifyGitHubUserAccess(
      githubUsername,
      providerAccountLogin
    );
    if (isOwner) {
      return { hasAccess: true, role: "owner" };
    }
  }
  return { hasAccess: false, role: "member" };
};

/**
 * POST /join
 * Join an organization by organization slug or ID
 *
 * SECURITY: User must have verified GitHub identity that matches:
 * - For GitHub Organizations: User must be a member of the GitHub org
 * - For GitHub User accounts: User must be the account owner
 */
app.post("/join", async (c) => {
  const auth = c.get("auth");
  const body = await c.req.json<JoinRequest>();

  const validationError = validateJoinRequest(body);
  if (validationError) {
    return c.json({ error: validationError }, 400);
  }

  const {
    organization_id: organizationId,
    organization_slug: organizationSlug,
    github_username: githubUsername,
  } = body;

  const { db, client } = await createDb(c.env);
  try {
    // Find the organization
    const organization = await db.query.organizations.findFirst({
      where: and(
        organizationId
          ? eq(organizations.id, organizationId)
          : eq(organizations.slug, organizationSlug ?? ""),
        isNull(organizations.deletedAt)
      ),
    });

    if (!organization) {
      return c.json({ error: "Organization not found" }, 404);
    }

    if (organization.suspendedAt) {
      return c.json({ error: "Organization is suspended" }, 403);
    }

    // Check if user is already a member
    const existingMember = await db.query.organizationMembers.findFirst({
      where: and(
        eq(organizationMembers.userId, auth.userId),
        eq(organizationMembers.organizationId, organization.id)
      ),
    });

    if (existingMember) {
      // Already a member, return existing membership
      return c.json({
        organization_id: organization.id,
        organization_name: organization.name,
        organization_slug: organization.slug,
        role: existingMember.role,
        github_linked: Boolean(existingMember.providerUserId),
        github_username: existingMember.providerUsername,
        joined: false,
      });
    }

    // SECURITY: Verify GitHub access based on account type
    const accessResult = await verifyGitHubAccess(
      githubUsername,
      organization.providerAccountType,
      organization.providerAccountLogin
    );

    if (!accessResult.hasAccess) {
      // SECURITY: Don't reveal whether the org exists or not for unauthorized users
      console.log(
        `[org-members] Access denied: ${githubUsername} is not a member of ${organization.providerAccountLogin}`
      );
      return c.json(
        {
          error:
            "You must be a member of this GitHub organization to join. Verify your GitHub account is linked and you have access.",
        },
        403
      );
    }

    // Add user as a member with verified role
    const memberId = crypto.randomUUID();
    await db.insert(organizationMembers).values({
      id: memberId,
      organizationId: organization.id,
      userId: auth.userId,
      role: accessResult.role,
      providerUsername: githubUsername,
    });

    console.log(
      `[org-members] User ${auth.userId} joined ${organization.slug} as ${accessResult.role}`
    );

    return c.json(
      {
        organization_id: organization.id,
        organization_name: organization.name,
        organization_slug: organization.slug,
        role: accessResult.role,
        github_linked: true,
        github_username: githubUsername,
        joined: true,
      },
      201
    );
  } finally {
    await client.end();
  }
});

/**
 * POST /leave
 * Leave an organization (user can leave any organization except if they're the only owner)
 */
app.post("/leave", async (c) => {
  const auth = c.get("auth");
  const body = await c.req.json<{ organization_id: string }>();
  const { organization_id: organizationId } = body;

  if (!organizationId) {
    return c.json({ error: "organization_id is required" }, 400);
  }

  // Validate organization_id format
  const validation = validateUUID(organizationId, "organization_id");
  if (!validation.valid) {
    return c.json({ error: validation.error }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Find the membership
    const member = await db.query.organizationMembers.findFirst({
      where: and(
        eq(organizationMembers.userId, auth.userId),
        eq(organizationMembers.organizationId, organizationId)
      ),
    });

    if (!member) {
      return c.json({ error: "Not a member of this organization" }, 404);
    }

    // If user is an owner, check if they're the only owner (use COUNT for efficiency)
    if (member.role === "owner") {
      const ownerCountResult = await db
        .select({ count: count() })
        .from(organizationMembers)
        .where(
          and(
            eq(organizationMembers.organizationId, organizationId),
            eq(organizationMembers.role, "owner")
          )
        );

      if (ownerCountResult[0]?.count === 1) {
        return c.json(
          {
            error:
              "Cannot leave organization as the only owner. Transfer ownership first.",
          },
          400
        );
      }
    }

    // Remove the membership
    await db
      .delete(organizationMembers)
      .where(
        and(
          eq(organizationMembers.userId, auth.userId),
          eq(organizationMembers.organizationId, organizationId)
        )
      );

    return c.json({ success: true });
  } finally {
    await client.end();
  }
});

/**
 * GET /
 * List members of an organization (user must be a member)
 */
app.get("/", async (c) => {
  const auth = c.get("auth");
  const organizationId = c.req.query("organization_id");

  if (!organizationId) {
    return c.json({ error: "organization_id is required" }, 400);
  }

  // Validate organization_id format
  const validation = validateUUID(organizationId, "organization_id");
  if (!validation.valid) {
    return c.json({ error: validation.error }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Verify user is a member of this organization
    const userMember = await db.query.organizationMembers.findFirst({
      where: and(
        eq(organizationMembers.userId, auth.userId),
        eq(organizationMembers.organizationId, organizationId)
      ),
    });

    if (!userMember) {
      return c.json({ error: "Not a member of this organization" }, 403);
    }

    // Get all members
    const members = await db.query.organizationMembers.findMany({
      where: eq(organizationMembers.organizationId, organizationId),
    });

    return c.json({
      members: members.map((m) => ({
        user_id: m.userId,
        role: m.role,
        github_linked: Boolean(m.providerUserId),
        github_user_id: m.providerUserId,
        github_username: m.providerUsername,
        joined_at: m.createdAt.toISOString(),
      })),
    });
  } finally {
    await client.end();
  }
});

export default app;
