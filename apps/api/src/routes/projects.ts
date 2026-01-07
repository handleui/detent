/**
 * Projects API routes
 *
 * Manages project registration - linking repositories to organizations.
 * Projects are created when users run `detent link` in their repo.
 */

import { and, eq, isNull } from "drizzle-orm";
import { Hono } from "hono";
import { createDb } from "../db/client";
import { organizationMembers, organizations, projects } from "../db/schema";
import { validateHandle, validateSlug, validateUUID } from "../lib/validation";
import type { Env } from "../types/env";

const app = new Hono<{ Bindings: Env }>();

/**
 * POST /
 * Register a project (link a repository to an organization)
 */
app.post("/", async (c) => {
  const auth = c.get("auth");
  const body = await c.req.json<{
    organization_id: string;
    provider_repo_id: string;
    provider_repo_name: string;
    provider_repo_full_name: string;
    provider_default_branch?: string;
    is_private?: boolean;
    handle?: string; // Optional custom handle, defaults to lowercase repo name
  }>();

  const {
    organization_id: organizationId,
    provider_repo_id: providerRepoId,
    provider_repo_name: providerRepoName,
    provider_repo_full_name: providerRepoFullName,
    provider_default_branch: providerDefaultBranch,
    is_private: isPrivate,
    handle: customHandle,
  } = body;

  if (
    !(
      organizationId &&
      providerRepoId &&
      providerRepoName &&
      providerRepoFullName
    )
  ) {
    return c.json(
      {
        error:
          "organization_id, provider_repo_id, provider_repo_name, and provider_repo_full_name are required",
      },
      400
    );
  }

  // Validate organization_id format
  const orgValidation = validateUUID(organizationId, "organization_id");
  if (!orgValidation.valid) {
    return c.json({ error: orgValidation.error }, 400);
  }

  // Validate custom handle if provided
  if (customHandle) {
    const handleValidation = validateHandle(customHandle, "handle");
    if (!handleValidation.valid) {
      return c.json({ error: handleValidation.error }, 400);
    }
  }

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

    // Check if organization is suspended or deleted
    if (member.organization.suspendedAt) {
      return c.json({ error: "Organization is suspended" }, 403);
    }

    if (member.organization.deletedAt) {
      return c.json({ error: "Organization has been deleted" }, 404);
    }

    // Check if project already exists for this repo in this organization
    const existingProject = await db.query.projects.findFirst({
      where: and(
        eq(projects.organizationId, organizationId),
        eq(projects.providerRepoId, providerRepoId),
        isNull(projects.removedAt)
      ),
    });

    if (existingProject) {
      // Project already exists, return it
      return c.json({
        project_id: existingProject.id,
        organization_id: existingProject.organizationId,
        handle: existingProject.handle,
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
    // Handle defaults to lowercase repo name for URL-friendly routing
    const handle =
      customHandle?.toLowerCase() ?? providerRepoName.toLowerCase();

    await db.insert(projects).values({
      id: projectId,
      organizationId,
      handle,
      providerRepoId,
      providerRepoName,
      providerRepoFullName,
      providerDefaultBranch: providerDefaultBranch ?? null,
      isPrivate: isPrivate ?? false,
    });

    return c.json(
      {
        project_id: projectId,
        organization_id: organizationId,
        handle,
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
 * List projects for an organization
 */
app.get("/", async (c) => {
  const auth = c.get("auth");
  const organizationId = c.req.query("organization_id");

  if (!organizationId) {
    return c.json({ error: "organization_id is required" }, 400);
  }

  // Validate organization_id format
  const orgValidation = validateUUID(organizationId, "organization_id");
  if (!orgValidation.valid) {
    return c.json({ error: orgValidation.error }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Verify user is a member of this organization
    const member = await db.query.organizationMembers.findFirst({
      where: and(
        eq(organizationMembers.userId, auth.userId),
        eq(organizationMembers.organizationId, organizationId)
      ),
    });

    if (!member) {
      return c.json({ error: "Not a member of this organization" }, 403);
    }

    // Get all active projects for this organization
    const organizationProjects = await db.query.projects.findMany({
      where: and(
        eq(projects.organizationId, organizationId),
        isNull(projects.removedAt)
      ),
    });

    return c.json({
      projects: organizationProjects.map((p) => ({
        project_id: p.id,
        organization_id: p.organizationId,
        handle: p.handle,
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
 * GET /lookup
 * Look up a project by repo full name
 * IMPORTANT: This route must be defined BEFORE /:projectId to prevent /lookup being treated as a projectId
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
      with: { organization: true },
    });

    if (!project) {
      return c.json({ error: "Project not found" }, 404);
    }

    // Verify user is a member of the project's organization
    const member = await db.query.organizationMembers.findFirst({
      where: and(
        eq(organizationMembers.userId, auth.userId),
        eq(organizationMembers.organizationId, project.organizationId)
      ),
    });

    if (!member) {
      return c.json({ error: "Not a member of this organization" }, 403);
    }

    return c.json({
      project_id: project.id,
      organization_id: project.organizationId,
      organization_name: project.organization.name,
      organization_slug: project.organization.slug,
      handle: project.handle,
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
 * GET /by-handle
 * Look up a project by organization slug and project handle
 * Enables @-style routing: @gh/handleui/api -> org=gh/handleui&handle=api
 */
app.get("/by-handle", async (c) => {
  const auth = c.get("auth");
  const orgSlug = c.req.query("org");
  const projectHandle = c.req.query("handle");

  if (!(orgSlug && projectHandle)) {
    return c.json(
      { error: "org and handle query parameters are required" },
      400
    );
  }

  // Validate slug and handle format
  const slugValidation = validateSlug(orgSlug, "org");
  if (!slugValidation.valid) {
    return c.json({ error: slugValidation.error }, 400);
  }

  const handleValidation = validateHandle(projectHandle, "handle");
  if (!handleValidation.valid) {
    return c.json({ error: handleValidation.error }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Single query: find org, project, and verify membership in parallel
    const org = await db.query.organizations.findFirst({
      where: and(
        eq(organizations.slug, orgSlug),
        isNull(organizations.deletedAt)
      ),
    });

    if (!org) {
      return c.json({ error: "Organization not found" }, 404);
    }

    // Fetch project and verify membership in parallel (both depend only on org.id)
    const [project, member] = await Promise.all([
      db.query.projects.findFirst({
        where: and(
          eq(projects.organizationId, org.id),
          eq(projects.handle, projectHandle.toLowerCase()),
          isNull(projects.removedAt)
        ),
      }),
      db.query.organizationMembers.findFirst({
        where: and(
          eq(organizationMembers.userId, auth.userId),
          eq(organizationMembers.organizationId, org.id)
        ),
      }),
    ]);

    if (!member) {
      return c.json({ error: "Not a member of this organization" }, 403);
    }

    if (!project) {
      return c.json({ error: "Project not found" }, 404);
    }

    return c.json({
      project_id: project.id,
      organization_id: org.id,
      organization_name: org.name,
      organization_slug: org.slug,
      handle: project.handle,
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
 * GET /:projectId
 * Get a specific project
 */
app.get("/:projectId", async (c) => {
  const auth = c.get("auth");
  const projectId = c.req.param("projectId");

  // Validate projectId format
  const validation = validateUUID(projectId, "projectId");
  if (!validation.valid) {
    return c.json({ error: validation.error }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Get the project
    const project = await db.query.projects.findFirst({
      where: and(eq(projects.id, projectId), isNull(projects.removedAt)),
      with: { organization: true },
    });

    if (!project) {
      return c.json({ error: "Project not found" }, 404);
    }

    // Verify user is a member of the project's organization
    const member = await db.query.organizationMembers.findFirst({
      where: and(
        eq(organizationMembers.userId, auth.userId),
        eq(organizationMembers.organizationId, project.organizationId)
      ),
    });

    if (!member) {
      return c.json({ error: "Not a member of this organization" }, 403);
    }

    return c.json({
      project_id: project.id,
      organization_id: project.organizationId,
      organization_name: project.organization.name,
      organization_slug: project.organization.slug,
      handle: project.handle,
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

  // Validate projectId format
  const validation = validateUUID(projectId, "projectId");
  if (!validation.valid) {
    return c.json({ error: validation.error }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Get the project
    const project = await db.query.projects.findFirst({
      where: and(eq(projects.id, projectId), isNull(projects.removedAt)),
    });

    if (!project) {
      return c.json({ error: "Project not found" }, 404);
    }

    // Verify user is a member of the project's organization with admin or owner role
    const member = await db.query.organizationMembers.findFirst({
      where: and(
        eq(organizationMembers.userId, auth.userId),
        eq(organizationMembers.organizationId, project.organizationId)
      ),
    });

    if (!member) {
      return c.json({ error: "Not a member of this organization" }, 403);
    }

    if (member.role === "member") {
      return c.json(
        { error: "Only organization owners and admins can remove projects" },
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
