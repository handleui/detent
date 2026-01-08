/**
 * Organization Members API routes
 *
 * SECURITY MODEL: GitHub is the single source of truth for membership.
 * Membership is verified on-demand via GitHub API using the App's installation token.
 * The organization_members table stores Detent-specific data (settings, audit, etc.)
 * but is NOT the source of truth for membership.
 *
 * The vulnerable /join endpoint has been REMOVED. Users access orgs by:
 * 1. Being a member of the GitHub org (verified on each request)
 * 2. Having their GitHub identity linked via WorkOS OAuth
 */

import { and, count, eq } from "drizzle-orm";
import { Hono } from "hono";
import { createDb } from "../db/client";
import { organizationMembers } from "../db/schema";
import { validateUUID } from "../lib/validation";
import {
  githubOrgAccessMiddleware,
  type OrgAccessContext,
} from "../middleware/github-org-access";
import type { Env } from "../types/env";

const app = new Hono<{ Bindings: Env }>();

/**
 * POST /leave
 * Leave an organization - removes user's Detent-specific record
 * Note: This doesn't remove them from the GitHub org, just clears Detent data
 */
app.post("/leave", async (c) => {
  const auth = c.get("auth");
  const body = await c.req.json<{ organization_id: string }>();
  const { organization_id: organizationId } = body;

  if (!organizationId) {
    return c.json({ error: "organization_id is required" }, 400);
  }

  const validation = validateUUID(organizationId, "organization_id");
  if (!validation.valid) {
    return c.json({ error: validation.error }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Find the membership record (if exists)
    const member = await db.query.organizationMembers.findFirst({
      where: and(
        eq(organizationMembers.userId, auth.userId),
        eq(organizationMembers.organizationId, organizationId)
      ),
    });

    if (!member) {
      // No Detent record exists, that's fine
      return c.json({
        success: true,
        message: "No membership record to remove",
      });
    }

    // If user is a Detent owner, check if they're the only one
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
              "Cannot leave as the only Detent owner. Transfer ownership first.",
          },
          400
        );
      }
    }

    // Remove the Detent membership record
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
 * GET /:orgId/members
 * List members of an organization
 * Uses on-demand GitHub membership verification via middleware
 */
app.get("/:orgId/members", githubOrgAccessMiddleware, async (c) => {
  const orgAccess = c.get("orgAccess") as OrgAccessContext;
  const { organization, githubIdentity, role } = orgAccess;

  const { db, client } = await createDb(c.env);
  try {
    // Get Detent-specific member records for this org
    const detentMembers = await db.query.organizationMembers.findMany({
      where: eq(organizationMembers.organizationId, organization.id),
    });

    // Return the current user's access plus any stored Detent records
    return c.json({
      current_user: {
        user_id: c.get("auth").userId,
        github_user_id: githubIdentity.userId,
        github_username: githubIdentity.username,
        role,
        is_installer: organization.installerGithubId === githubIdentity.userId,
      },
      // Detent-specific records (may not include all GitHub org members)
      detent_members: detentMembers.map((m) => ({
        user_id: m.userId,
        role: m.role,
        github_user_id: m.providerUserId,
        github_username: m.providerUsername,
        github_linked: Boolean(m.providerUserId),
        joined_at: m.createdAt.toISOString(),
      })),
      // Note: For a full list of GitHub org members, query GitHub API directly
      note: "This list shows Detent users who have accessed this org. GitHub org membership is verified on-demand.",
    });
  } finally {
    await client.end();
  }
});

/**
 * GET /:orgId/me
 * Get current user's access to an organization
 * Verifies GitHub membership on-demand
 */
app.get("/:orgId/me", githubOrgAccessMiddleware, async (c) => {
  const orgAccess = c.get("orgAccess") as OrgAccessContext;
  const { organization, githubIdentity, role } = orgAccess;
  const auth = c.get("auth");

  const { db, client } = await createDb(c.env);
  try {
    // Check if user has a Detent record
    const detentRecord = await db.query.organizationMembers.findFirst({
      where: and(
        eq(organizationMembers.userId, auth.userId),
        eq(organizationMembers.organizationId, organization.id)
      ),
    });

    // Optionally create/update Detent record for audit purposes
    if (!detentRecord) {
      // First access - create a Detent record
      await db.insert(organizationMembers).values({
        id: crypto.randomUUID(),
        organizationId: organization.id,
        userId: auth.userId,
        role,
        providerUserId: githubIdentity.userId,
        providerUsername: githubIdentity.username,
        providerLinkedAt: new Date(),
      });
    } else if (
      detentRecord.providerUserId !== githubIdentity.userId ||
      detentRecord.providerUsername !== githubIdentity.username
    ) {
      // Update GitHub identity if changed
      await db
        .update(organizationMembers)
        .set({
          providerUserId: githubIdentity.userId,
          providerUsername: githubIdentity.username,
          providerLinkedAt: new Date(),
          updatedAt: new Date(),
        })
        .where(eq(organizationMembers.id, detentRecord.id));
    }

    return c.json({
      organization_id: organization.id,
      organization_name: organization.name,
      organization_slug: organization.slug,
      role,
      github_user_id: githubIdentity.userId,
      github_username: githubIdentity.username,
      is_installer: organization.installerGithubId === githubIdentity.userId,
      provider: organization.provider,
      provider_account: organization.providerAccountLogin,
    });
  } finally {
    await client.end();
  }
});

/**
 * GET /by-org/:orgId
 * Legacy endpoint - redirect to new pattern
 */
app.get("/by-org/:orgId", (c) => {
  const orgId = c.req.param("orgId");
  return c.redirect(`/v1/organization-members/${orgId}/members`, 301);
});

export default app;
