/**
 * Organizations API routes
 *
 * Handles organization-specific operations like status and details.
 */

import { and, count, eq, inArray, isNull } from "drizzle-orm";
import { Hono } from "hono";
import { createDb } from "../db/client";
import { organizationMembers, organizations, projects } from "../db/schema";
import { validateUUID } from "../lib/validation";
import { createGitHubService } from "../services/github";
import type { Env } from "../types/env";

interface SyncResult {
  added: number;
  removed: number;
  updated: number;
}

interface GitHubRepo {
  id: number;
  name: string;
  full_name: string;
  default_branch: string;
  private: boolean;
}

interface ProjectSnapshot {
  id: string;
  providerRepoId: string;
  providerRepoName: string;
  providerRepoFullName: string;
  isPrivate: boolean;
  removedAt: Date | null;
}

type DbClient = Awaited<ReturnType<typeof createDb>>["db"];

/**
 * Process repos that need to be added or reactivated
 */
const processReposToAdd = async (
  db: DbClient,
  reposToAdd: GitHubRepo[],
  projectsByRepoId: Map<string, ProjectSnapshot>,
  organizationId: string
): Promise<{ added: number; updated: number }> => {
  let added = 0;
  let updated = 0;

  for (const repo of reposToAdd) {
    const existing = projectsByRepoId.get(String(repo.id));
    if (existing?.removedAt) {
      await db
        .update(projects)
        .set({
          removedAt: null,
          providerRepoName: repo.name,
          providerRepoFullName: repo.full_name,
          providerDefaultBranch: repo.default_branch,
          isPrivate: repo.private,
          updatedAt: new Date(),
        })
        .where(eq(projects.id, existing.id));
      updated++;
    } else {
      await db.insert(projects).values({
        id: crypto.randomUUID(),
        organizationId,
        handle: repo.name.toLowerCase(),
        providerRepoId: String(repo.id),
        providerRepoName: repo.name,
        providerRepoFullName: repo.full_name,
        providerDefaultBranch: repo.default_branch,
        isPrivate: repo.private,
      });
      added++;
    }
  }

  return { added, updated };
};

/**
 * Update projects that exist in both GitHub and our DB but have changed
 */
const updateChangedProjects = async (
  db: DbClient,
  githubRepos: GitHubRepo[],
  projectsByRepoId: Map<string, ProjectSnapshot>
): Promise<number> => {
  let updated = 0;

  for (const repo of githubRepos) {
    const existing = projectsByRepoId.get(String(repo.id));
    const hasChanges =
      existing &&
      !existing.removedAt &&
      (existing.providerRepoName !== repo.name ||
        existing.providerRepoFullName !== repo.full_name ||
        existing.isPrivate !== repo.private);

    if (hasChanges && existing) {
      await db
        .update(projects)
        .set({
          providerRepoName: repo.name,
          providerRepoFullName: repo.full_name,
          providerDefaultBranch: repo.default_branch,
          isPrivate: repo.private,
          updatedAt: new Date(),
        })
        .where(eq(projects.id, existing.id));
      updated++;
    }
  }

  return updated;
};

const app = new Hono<{ Bindings: Env }>();

/**
 * GET /:organizationId/status
 * Get detailed status of an organization including GitHub App installation
 */
app.get("/:organizationId/status", async (c) => {
  const auth = c.get("auth");
  const organizationId = c.req.param("organizationId");

  // Validate organizationId format
  const validation = validateUUID(organizationId, "organizationId");
  if (!validation.valid) {
    return c.json({ error: validation.error }, 400);
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
      last_synced_at: organization.lastSyncedAt?.toISOString() ?? null,
    });
  } finally {
    await client.end();
  }
});

/**
 * POST /:organizationId/sync
 * Sync organization state with GitHub (check installation, update repos)
 * This reconciles our database with the current GitHub state
 */
app.post("/:organizationId/sync", async (c) => {
  const auth = c.get("auth");
  const organizationId = c.req.param("organizationId");

  // Validate organizationId format
  const validation = validateUUID(organizationId, "organizationId");
  if (!validation.valid) {
    return c.json({ error: validation.error }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Verify user is an admin or owner of this organization
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

    if (member.role === "member") {
      return c.json(
        { error: "Only organization owners and admins can trigger sync" },
        403
      );
    }

    const { organization } = member;

    if (organization.deletedAt) {
      return c.json({ error: "Organization has been deleted" }, 404);
    }

    // Only GitHub organizations can be synced (GitLab uses different mechanism)
    if (organization.provider !== "github") {
      return c.json(
        { error: "Sync only supported for GitHub organizations" },
        400
      );
    }

    if (!organization.providerInstallationId) {
      return c.json({ error: "No GitHub App installation found" }, 400);
    }

    const github = createGitHubService(c.env);
    const installationId = Number(organization.providerInstallationId);

    // 1. Check if installation still exists and get its status
    const installationInfo = await github.getInstallationInfo(installationId);

    if (!installationInfo) {
      // Installation was removed - mark organization as deleted
      await db
        .update(organizations)
        .set({
          deletedAt: new Date(),
          lastSyncedAt: new Date(),
          updatedAt: new Date(),
        })
        .where(eq(organizations.id, organizationId));

      return c.json({
        message: "installation_removed",
        organization_id: organizationId,
        synced: true,
      });
    }

    // 2. Update suspension status if changed
    const wasSuspended = Boolean(organization.suspendedAt);
    const isSuspended = Boolean(installationInfo.suspended_at);

    if (wasSuspended !== isSuspended) {
      await db
        .update(organizations)
        .set({
          suspendedAt: isSuspended
            ? new Date(installationInfo.suspended_at as string)
            : null,
          updatedAt: new Date(),
        })
        .where(eq(organizations.id, organizationId));
    }

    // 3. Get current repos from GitHub and reconcile with our projects
    const githubRepos = await github.getInstallationRepos(installationId);
    const githubRepoIds = new Set(githubRepos.map((r) => String(r.id)));

    // Get our current active projects
    const ourProjects = await db
      .select({
        id: projects.id,
        providerRepoId: projects.providerRepoId,
        providerRepoName: projects.providerRepoName,
        providerRepoFullName: projects.providerRepoFullName,
        isPrivate: projects.isPrivate,
        removedAt: projects.removedAt,
      })
      .from(projects)
      .where(eq(projects.organizationId, organizationId));

    const ourProjectsByRepoId = new Map(
      ourProjects.map((p) => [p.providerRepoId, p])
    );

    const result: SyncResult = { added: 0, removed: 0, updated: 0 };

    // Find repos to add (in GitHub but not in our DB or were soft-deleted)
    const reposToAdd = githubRepos.filter((repo) => {
      const existing = ourProjectsByRepoId.get(String(repo.id));
      return !existing || existing.removedAt;
    });

    // Process repos to add/reactivate
    if (reposToAdd.length > 0) {
      const addResult = await processReposToAdd(
        db,
        reposToAdd,
        ourProjectsByRepoId,
        organizationId
      );
      result.added = addResult.added;
      result.updated += addResult.updated;
    }

    // Find repos to remove (in our DB but no longer in GitHub)
    const projectsToRemove = ourProjects.filter(
      (p) => !(p.removedAt || githubRepoIds.has(p.providerRepoId))
    );

    if (projectsToRemove.length > 0) {
      const idsToRemove = projectsToRemove.map((p) => p.id);
      await db
        .update(projects)
        .set({ removedAt: new Date(), updatedAt: new Date() })
        .where(inArray(projects.id, idsToRemove));
      result.removed = projectsToRemove.length;
    }

    // Update projects that exist in both (check for name/visibility changes)
    const changedCount = await updateChangedProjects(
      db,
      githubRepos,
      ourProjectsByRepoId
    );
    result.updated += changedCount;

    // 4. Update lastSyncedAt
    await db
      .update(organizations)
      .set({ lastSyncedAt: new Date(), updatedAt: new Date() })
      .where(eq(organizations.id, organizationId));

    return c.json({
      message: "sync completed",
      organization_id: organizationId,
      suspended: isSuspended,
      projects_added: result.added,
      projects_removed: result.removed,
      projects_updated: result.updated,
      total_repos: githubRepos.length,
    });
  } catch (error) {
    console.error("[organizations/sync] Error:", error);
    return c.json(
      {
        message: "sync error",
        error: error instanceof Error ? error.message : "Unknown error",
      },
      500
    );
  } finally {
    await client.end();
  }
});

export default app;
