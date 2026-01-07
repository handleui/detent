/**
 * Organizations API routes
 *
 * Handles organization-specific operations like status and details.
 */

import { and, count, eq, isNull } from "drizzle-orm";
import { Hono } from "hono";
import { createDb } from "../db/client";
import { organizationMembers, projects } from "../db/schema";
import type { Env } from "../types/env";

const app = new Hono<{ Bindings: Env }>();

/**
 * GET /:organizationId/status
 * Get detailed status of an organization including GitHub App installation
 */
app.get("/:organizationId/status", async (c) => {
  const auth = c.get("auth");
  const organizationId = c.req.param("organizationId");

  const { db, client } = await createDb(c.env);
  try {
    // Verify user is a member of this organization
    const member = await db.query.organizationMembers.findFirst({
      where: and(
        eq(organizationMembers.userId, auth.userId),
        eq(organizationMembers.organizationId, organizationId)
      ),
      with: { organization: true },
    });

    if (!member) {
      return c.json({ error: "Not a member of this organization" }, 403);
    }

    const { organization } = member;

    if (organization.deletedAt) {
      return c.json({ error: "Organization has been deleted" }, 404);
    }

    // Count active projects efficiently using SQL COUNT
    const projectCountResult = await db
      .select({ count: count() })
      .from(projects)
      .where(
        and(
          eq(projects.organizationId, organizationId),
          isNull(projects.removedAt)
        )
      );

    const appInstalled = Boolean(organization.providerInstallationId);
    const projectCount = projectCountResult[0]?.count ?? 0;

    return c.json({
      organization_id: organization.id,
      organization_name: organization.name,
      organization_slug: organization.slug,
      provider: organization.provider,
      provider_account_login: organization.providerAccountLogin,
      provider_account_type: organization.providerAccountType,
      app_installed: appInstalled,
      suspended_at: organization.suspendedAt?.toISOString() ?? null,
      project_count: projectCount,
      created_at: organization.createdAt.toISOString(),
    });
  } finally {
    await client.end();
  }
});

export default app;
