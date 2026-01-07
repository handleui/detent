/**
 * Projects API routes
 *
 * Manages project registration - linking repositories to teams.
 * Projects are created when users run `detent link` in their repo.
 */

import { and, eq, isNull } from "drizzle-orm";
import { Hono } from "hono";
import { createDb } from "../db/client";
import { projects, teamMembers } from "../db/schema";
import type { Env } from "../types/env";

const app = new Hono<{ Bindings: Env }>();

/**
 * POST /
 * Register a project (link a repository to a team)
 */
app.post("/", async (c) => {
  const auth = c.get("auth");
  const body = await c.req.json<{
    team_id: string;
    provider_repo_id: string;
    provider_repo_name: string;
    provider_repo_full_name: string;
    provider_default_branch?: string;
    is_private?: boolean;
  }>();

  const {
    team_id: teamId,
    provider_repo_id: providerRepoId,
    provider_repo_name: providerRepoName,
    provider_repo_full_name: providerRepoFullName,
    provider_default_branch: providerDefaultBranch,
    is_private: isPrivate,
  } = body;

  if (!(teamId && providerRepoId && providerRepoName && providerRepoFullName)) {
    return c.json(
      {
        error:
          "team_id, provider_repo_id, provider_repo_name, and provider_repo_full_name are required",
      },
      400
    );
  }

  const { db, client } = await createDb(c.env);
  try {
    // Verify user is a member of this team
    const member = await db.query.teamMembers.findFirst({
      where: and(
        eq(teamMembers.userId, auth.userId),
        eq(teamMembers.teamId, teamId)
      ),
      with: { team: true },
    });

    if (!member) {
      return c.json({ error: "Not a member of this team" }, 403);
    }

    // Check if team is suspended or deleted
    if (member.team.suspendedAt) {
      return c.json({ error: "Team is suspended" }, 403);
    }

    if (member.team.deletedAt) {
      return c.json({ error: "Team has been deleted" }, 404);
    }

    // Check if project already exists for this repo in this team
    const existingProject = await db.query.projects.findFirst({
      where: and(
        eq(projects.teamId, teamId),
        eq(projects.providerRepoId, providerRepoId),
        isNull(projects.removedAt)
      ),
    });

    if (existingProject) {
      // Project already exists, return it
      return c.json({
        project_id: existingProject.id,
        team_id: existingProject.teamId,
        provider_repo_id: existingProject.providerRepoId,
        provider_repo_name: existingProject.providerRepoName,
        provider_repo_full_name: existingProject.providerRepoFullName,
        provider_default_branch: existingProject.providerDefaultBranch,
        is_private: existingProject.isPrivate,
        created: false,
      });
    }

    // Create the project
    const projectId = crypto.randomUUID();

    await db.insert(projects).values({
      id: projectId,
      teamId,
      providerRepoId,
      providerRepoName,
      providerRepoFullName,
      providerDefaultBranch: providerDefaultBranch ?? null,
      isPrivate: isPrivate ?? false,
    });

    return c.json(
      {
        project_id: projectId,
        team_id: teamId,
        provider_repo_id: providerRepoId,
        provider_repo_name: providerRepoName,
        provider_repo_full_name: providerRepoFullName,
        provider_default_branch: providerDefaultBranch ?? null,
        is_private: isPrivate ?? false,
        created: true,
      },
      201
    );
  } finally {
    await client.end();
  }
});

/**
 * GET /
 * List projects for a team
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
    const member = await db.query.teamMembers.findFirst({
      where: and(
        eq(teamMembers.userId, auth.userId),
        eq(teamMembers.teamId, teamId)
      ),
    });

    if (!member) {
      return c.json({ error: "Not a member of this team" }, 403);
    }

    // Get all active projects for this team
    const teamProjects = await db.query.projects.findMany({
      where: and(eq(projects.teamId, teamId), isNull(projects.removedAt)),
    });

    return c.json({
      projects: teamProjects.map((p) => ({
        project_id: p.id,
        team_id: p.teamId,
        provider_repo_id: p.providerRepoId,
        provider_repo_name: p.providerRepoName,
        provider_repo_full_name: p.providerRepoFullName,
        provider_default_branch: p.providerDefaultBranch,
        is_private: p.isPrivate,
        created_at: p.createdAt.toISOString(),
      })),
    });
  } finally {
    await client.end();
  }
});

/**
 * GET /:projectId
 * Get a specific project
 */
app.get("/:projectId", async (c) => {
  const auth = c.get("auth");
  const projectId = c.req.param("projectId");

  const { db, client } = await createDb(c.env);
  try {
    // Get the project
    const project = await db.query.projects.findFirst({
      where: and(eq(projects.id, projectId), isNull(projects.removedAt)),
      with: { team: true },
    });

    if (!project) {
      return c.json({ error: "Project not found" }, 404);
    }

    // Verify user is a member of the project's team
    const member = await db.query.teamMembers.findFirst({
      where: and(
        eq(teamMembers.userId, auth.userId),
        eq(teamMembers.teamId, project.teamId)
      ),
    });

    if (!member) {
      return c.json({ error: "Not a member of this team" }, 403);
    }

    return c.json({
      project_id: project.id,
      team_id: project.teamId,
      team_name: project.team.name,
      team_slug: project.team.slug,
      provider_repo_id: project.providerRepoId,
      provider_repo_name: project.providerRepoName,
      provider_repo_full_name: project.providerRepoFullName,
      provider_default_branch: project.providerDefaultBranch,
      is_private: project.isPrivate,
      created_at: project.createdAt.toISOString(),
    });
  } finally {
    await client.end();
  }
});

/**
 * GET /lookup
 * Look up a project by repo full name
 */
app.get("/lookup", async (c) => {
  const auth = c.get("auth");
  const repoFullName = c.req.query("repo");

  if (!repoFullName) {
    return c.json({ error: "repo query parameter is required" }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Find project by repo full name
    const project = await db.query.projects.findFirst({
      where: and(
        eq(projects.providerRepoFullName, repoFullName),
        isNull(projects.removedAt)
      ),
      with: { team: true },
    });

    if (!project) {
      return c.json({ error: "Project not found" }, 404);
    }

    // Verify user is a member of the project's team
    const member = await db.query.teamMembers.findFirst({
      where: and(
        eq(teamMembers.userId, auth.userId),
        eq(teamMembers.teamId, project.teamId)
      ),
    });

    if (!member) {
      return c.json({ error: "Not a member of this team" }, 403);
    }

    return c.json({
      project_id: project.id,
      team_id: project.teamId,
      team_name: project.team.name,
      team_slug: project.team.slug,
      provider_repo_id: project.providerRepoId,
      provider_repo_name: project.providerRepoName,
      provider_repo_full_name: project.providerRepoFullName,
      provider_default_branch: project.providerDefaultBranch,
      is_private: project.isPrivate,
      created_at: project.createdAt.toISOString(),
    });
  } finally {
    await client.end();
  }
});

/**
 * DELETE /:projectId
 * Remove a project (soft delete)
 */
app.delete("/:projectId", async (c) => {
  const auth = c.get("auth");
  const projectId = c.req.param("projectId");

  const { db, client } = await createDb(c.env);
  try {
    // Get the project
    const project = await db.query.projects.findFirst({
      where: and(eq(projects.id, projectId), isNull(projects.removedAt)),
    });

    if (!project) {
      return c.json({ error: "Project not found" }, 404);
    }

    // Verify user is a member of the project's team with admin or owner role
    const member = await db.query.teamMembers.findFirst({
      where: and(
        eq(teamMembers.userId, auth.userId),
        eq(teamMembers.teamId, project.teamId)
      ),
    });

    if (!member) {
      return c.json({ error: "Not a member of this team" }, 403);
    }

    if (member.role === "member") {
      return c.json(
        { error: "Only team owners and admins can remove projects" },
        403
      );
    }

    // Soft delete the project
    await db
      .update(projects)
      .set({
        removedAt: new Date(),
        updatedAt: new Date(),
      })
      .where(eq(projects.id, projectId));

    return c.json({ success: true });
  } finally {
    await client.end();
  }
});

export default app;
