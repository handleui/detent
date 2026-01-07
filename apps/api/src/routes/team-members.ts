/**
 * Team Members API routes
 *
 * Manages team membership - adding/removing users from teams.
 * Users can join teams via the GitHub App installation flow.
 */

import { and, eq, isNull } from "drizzle-orm";
import { Hono } from "hono";
import { createDb } from "../db/client";
import { teamMembers, teams } from "../db/schema";
import type { Env } from "../types/env";

const app = new Hono<{ Bindings: Env }>();

/**
 * POST /join
 * Join a team by team slug or ID
 * User must be authenticated and the team must exist
 */
app.post("/join", async (c) => {
  const auth = c.get("auth");
  const body = await c.req.json<{
    team_id?: string;
    team_slug?: string;
  }>();

  const { team_id: teamId, team_slug: teamSlug } = body;

  if (!(teamId || teamSlug)) {
    return c.json({ error: "team_id or team_slug is required" }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Find the team
    const team = await db.query.teams.findFirst({
      where: and(
        teamId ? eq(teams.id, teamId) : eq(teams.slug, teamSlug ?? ""),
        isNull(teams.deletedAt)
      ),
    });

    if (!team) {
      return c.json({ error: "Team not found" }, 404);
    }

    if (team.suspendedAt) {
      return c.json({ error: "Team is suspended" }, 403);
    }

    // Check if user is already a member
    const existingMember = await db.query.teamMembers.findFirst({
      where: and(
        eq(teamMembers.userId, auth.userId),
        eq(teamMembers.teamId, team.id)
      ),
    });

    if (existingMember) {
      // Already a member, return existing membership
      return c.json({
        team_id: team.id,
        team_name: team.name,
        team_slug: team.slug,
        role: existingMember.role,
        github_linked: Boolean(existingMember.providerUserId),
        github_username: existingMember.providerUsername,
        joined: false,
      });
    }

    // Add user as a member (default role)
    const memberId = crypto.randomUUID();
    await db.insert(teamMembers).values({
      id: memberId,
      teamId: team.id,
      userId: auth.userId,
      role: "member",
    });

    return c.json(
      {
        team_id: team.id,
        team_name: team.name,
        team_slug: team.slug,
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
 * Leave a team (user can leave any team except if they're the only owner)
 */
app.post("/leave", async (c) => {
  const auth = c.get("auth");
  const body = await c.req.json<{ team_id: string }>();
  const { team_id: teamId } = body;

  if (!teamId) {
    return c.json({ error: "team_id is required" }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Find the membership
    const member = await db.query.teamMembers.findFirst({
      where: and(
        eq(teamMembers.userId, auth.userId),
        eq(teamMembers.teamId, teamId)
      ),
    });

    if (!member) {
      return c.json({ error: "Not a member of this team" }, 404);
    }

    // If user is an owner, check if they're the only owner
    if (member.role === "owner") {
      const owners = await db.query.teamMembers.findMany({
        where: and(
          eq(teamMembers.teamId, teamId),
          eq(teamMembers.role, "owner")
        ),
      });

      if (owners.length === 1) {
        return c.json(
          {
            error:
              "Cannot leave team as the only owner. Transfer ownership first.",
          },
          400
        );
      }
    }

    // Remove the membership
    await db
      .delete(teamMembers)
      .where(
        and(eq(teamMembers.userId, auth.userId), eq(teamMembers.teamId, teamId))
      );

    return c.json({ success: true });
  } finally {
    await client.end();
  }
});

/**
 * GET /
 * List members of a team (user must be a member)
 */
app.get("/", async (c) => {
  const auth = c.get("auth");
  const teamId = c.req.query("team_id");

  if (!teamId) {
    return c.json({ error: "team_id is required" }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Verify user is a member of this team
    const userMember = await db.query.teamMembers.findFirst({
      where: and(
        eq(teamMembers.userId, auth.userId),
        eq(teamMembers.teamId, teamId)
      ),
    });

    if (!userMember) {
      return c.json({ error: "Not a member of this team" }, 403);
    }

    // Get all members
    const members = await db.query.teamMembers.findMany({
      where: eq(teamMembers.teamId, teamId),
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
