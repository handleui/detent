/**
 * Organization Members API routes
 *
 * Manages organization membership - adding/removing users from organizations.
 * Users can join organizations via the GitHub App installation flow.
 */

import { and, eq, isNull } from "drizzle-orm";
import { Hono } from "hono";
import { createDb } from "../db/client";
import { organizationMembers, organizations } from "../db/schema";
import type { Env } from "../types/env";

const app = new Hono<{ Bindings: Env }>();

/**
 * POST /join
 * Join an organization by organization slug or ID
 * User must be authenticated and the organization must exist
 */
app.post("/join", async (c) => {
  const auth = c.get("auth");
  const body = await c.req.json<{
    organization_id?: string;
    organization_slug?: string;
  }>();

  const {
    organization_id: organizationId,
    organization_slug: organizationSlug,
  } = body;

  if (!(organizationId || organizationSlug)) {
    return c.json(
      { error: "organization_id or organization_slug is required" },
      400
    );
  }

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

    // Add user as a member (default role)
    const memberId = crypto.randomUUID();
    await db.insert(organizationMembers).values({
      id: memberId,
      organizationId: organization.id,
      userId: auth.userId,
      role: "member",
    });

    return c.json(
      {
        organization_id: organization.id,
        organization_name: organization.name,
        organization_slug: organization.slug,
        role: "member",
        github_linked: false,
        github_username: null,
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

    // If user is an owner, check if they're the only owner
    if (member.role === "owner") {
      const owners = await db.query.organizationMembers.findMany({
        where: and(
          eq(organizationMembers.organizationId, organizationId),
          eq(organizationMembers.role, "owner")
        ),
      });

      if (owners.length === 1) {
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
