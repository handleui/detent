import { and, eq, inArray, isNotNull } from "drizzle-orm";
import type { Context } from "hono";
import { Hono } from "hono";
import { createDb } from "../db/client";
import {
  createProviderSlug,
  organizationMembers,
  organizations,
  projects,
} from "../db/schema";
import { webhookSignatureMiddleware } from "../middleware/webhook-signature";
import { createGitHubService } from "../services/github";
import type { Env } from "../types/env";

// Type definitions for GitHub webhook payloads
interface WorkflowRunPayload {
  action: string;
  workflow_run: {
    id: number;
    name: string;
    conclusion: string | null;
    head_branch: string;
    pull_requests: Array<{ number: number }>;
  };
  repository: {
    full_name: string;
    owner: { login: string };
    name: string;
  };
  installation: { id: number };
}

interface IssueCommentPayload {
  action: string;
  comment: {
    body: string;
    user: { login: string };
  };
  issue: {
    number: number;
    pull_request?: { url: string };
  };
  repository: {
    full_name: string;
    owner: { login: string };
    name: string;
  };
  installation: { id: number };
}

interface PingPayload {
  zen: string;
  hook_id: number;
}

interface InstallationPayload {
  action:
    | "created"
    | "deleted"
    | "suspend"
    | "unsuspend"
    | "new_permissions_accepted";
  installation: {
    id: number;
    account: {
      id: number;
      login: string;
      type: "Organization" | "User";
      avatar_url?: string;
    };
  };
  // The user who triggered the webhook event (installer for "created" action)
  sender: {
    id: number;
    login: string;
    type: "User";
  };
  repositories?: Array<{
    id: number;
    name: string;
    full_name: string;
    private: boolean;
  }>;
}

interface InstallationRepositoriesPayload {
  action: "added" | "removed";
  installation: {
    id: number;
    account: {
      id: number;
      login: string;
      type: "Organization" | "User";
    };
  };
  repositories_added: Array<{
    id: number;
    name: string;
    full_name: string;
    private: boolean;
  }>;
  repositories_removed: Array<{
    id: number;
    name: string;
    full_name: string;
    private: boolean;
  }>;
}

interface RepositoryPayload {
  action: "renamed" | "transferred" | "privatized" | "publicized";
  repository: {
    id: number;
    name: string;
    full_name: string;
    private: boolean;
    default_branch?: string;
  };
  changes?: {
    repository?: {
      name?: { from: string };
    };
  };
  installation?: { id: number };
}

interface OrganizationPayload {
  action: "renamed" | "member_added" | "member_removed" | "member_invited";
  organization: {
    id: number;
    login: string;
    avatar_url?: string;
  };
  changes?: {
    login?: {
      from: string;
    };
  };
  installation?: { id: number };
}

interface DetentCommand {
  type: "heal" | "status" | "help" | "unknown";
  dryRun?: boolean;
}

// Variables stored in context by middleware
interface WebhookVariables {
  webhookPayload:
    | WorkflowRunPayload
    | IssueCommentPayload
    | PingPayload
    | InstallationPayload
    | InstallationRepositoriesPayload
    | RepositoryPayload
    | OrganizationPayload;
}

type WebhookContext = Context<{ Bindings: Env; Variables: WebhookVariables }>;

const app = new Hono<{ Bindings: Env; Variables: WebhookVariables }>();

// GitHub webhook endpoint
// Receives: workflow_run, issue_comment events
app.post("/github", webhookSignatureMiddleware, (c: WebhookContext) => {
  const event = c.req.header("X-GitHub-Event");
  const deliveryId = c.req.header("X-GitHub-Delivery");
  const payload = c.get("webhookPayload");

  console.log(`[webhook] Received ${event} event (delivery: ${deliveryId})`);

  // Route by event type
  switch (event) {
    case "workflow_run":
      return handleWorkflowRunEvent(c, payload as WorkflowRunPayload);

    case "issue_comment":
      return handleIssueCommentEvent(c, payload as IssueCommentPayload);

    case "ping":
      // GitHub sends this when webhook is first configured
      return c.json({ message: "pong", zen: (payload as PingPayload).zen });

    case "installation":
      return handleInstallationEvent(c, payload as InstallationPayload);

    case "installation_repositories":
      return handleInstallationRepositoriesEvent(
        c,
        payload as InstallationRepositoriesPayload
      );

    case "repository":
      return handleRepositoryEvent(c, payload as RepositoryPayload);

    case "organization":
      return handleOrganizationEvent(c, payload as OrganizationPayload);

    default:
      console.log(`[webhook] Ignoring unhandled event: ${event}`);
      return c.json({ message: "ignored", event });
  }
});

// Handle workflow_run events (CI completed)
const handleWorkflowRunEvent = async (
  c: WebhookContext,
  payload: WorkflowRunPayload
) => {
  const { action, workflow_run, repository, installation } = payload;

  // Only process completed runs
  if (action !== "completed") {
    return c.json({ message: "ignored", reason: "not completed" });
  }

  // Only process failures
  if (workflow_run.conclusion !== "failure") {
    return c.json({ message: "ignored", reason: "not a failure" });
  }

  console.log(
    `[workflow_run] Failed: ${repository.full_name} / ${workflow_run.name} (run ${workflow_run.id})`
  );

  // Get GitHub service
  const github = createGitHubService(c.env);

  try {
    // 1. Get installation token
    const token = await github.getInstallationToken(installation.id);

    // 2. Check PR and fetch logs in parallel (both only need the token)
    const prFromPayload = workflow_run.pull_requests[0]?.number;
    const [prFromApi, logs] = await Promise.all([
      // Only fetch PR from API if not in payload
      prFromPayload
        ? Promise.resolve(null)
        : github.getPullRequestForRun(
            token,
            repository.owner.login,
            repository.name,
            workflow_run.id
          ),
      github.fetchWorkflowLogs(
        token,
        repository.owner.login,
        repository.name,
        workflow_run.id
      ),
    ]);

    const prNumber = prFromPayload ?? prFromApi;

    if (!prNumber) {
      console.log("[workflow_run] No associated PR found, skipping comment");
      return c.json({
        message: "workflow_run processed",
        repository: repository.full_name,
        runId: workflow_run.id,
        status: "no_pr",
      });
    }

    // Future: Parse errors with @detent/parser and include them in the comment
    // const errors = parseWorkflowLogs(logs);

    // 4. Post summary comment on PR
    const commentBody = formatFailureComment(
      repository.owner.login,
      repository.name,
      workflow_run.name,
      workflow_run.id,
      logs
    );

    await github.postComment(
      token,
      repository.owner.login,
      repository.name,
      prNumber,
      commentBody
    );

    return c.json({
      message: "workflow_run processed",
      repository: repository.full_name,
      runId: workflow_run.id,
      prNumber,
      status: "commented",
    });
  } catch (error) {
    console.error("[workflow_run] Error processing:", error);
    return c.json(
      {
        message: "workflow_run error",
        error: error instanceof Error ? error.message : "Unknown error",
      },
      500
    );
  }
};

// Generate a unique slug for an organization
type DbClient = Awaited<ReturnType<typeof createDb>>["db"];

// Auto-link installer to organization if they have an existing Detent account
const autoLinkInstaller = async (
  db: DbClient,
  organizationId: string,
  installerGithubId: string,
  installerUsername: string
): Promise<boolean> => {
  // Check if installer already has a Detent account (via any org membership with matching GitHub ID)
  const existingMember = await db
    .select({
      userId: organizationMembers.userId,
    })
    .from(organizationMembers)
    .where(eq(organizationMembers.providerUserId, installerGithubId))
    .limit(1);

  if (!existingMember[0]) {
    // User doesn't exist in Detent system yet - will be linked via sync-identity endpoint later
    return false;
  }

  // Check if they already have membership to this specific org
  const existingMembership = await db
    .select({ id: organizationMembers.id })
    .from(organizationMembers)
    .where(
      and(
        eq(organizationMembers.userId, existingMember[0].userId),
        eq(organizationMembers.organizationId, organizationId)
      )
    )
    .limit(1);

  if (existingMembership[0]) {
    // Already a member of this org
    console.log(
      `[webhook] Installer ${installerGithubId} already has membership to org ${organizationId}`
    );
    return false;
  }

  // Create owner membership for the installer
  await db.insert(organizationMembers).values({
    id: crypto.randomUUID(),
    organizationId,
    userId: existingMember[0].userId,
    role: "owner",
    providerUserId: installerGithubId,
    providerUsername: installerUsername,
    providerLinkedAt: new Date(),
  });

  console.log(
    `[webhook] Auto-linked installer ${installerGithubId} (${installerUsername}) as owner to org ${organizationId}`
  );
  return true;
};

const generateUniqueSlug = async (
  db: DbClient,
  baseSlug: string
): Promise<string> => {
  const maxSlugAttempts = 10;

  // Generate all potential slugs upfront: baseSlug, baseSlug-1, baseSlug-2, ...
  const potentialSlugs = [
    baseSlug,
    ...Array.from(
      { length: maxSlugAttempts },
      (_, i) => `${baseSlug}-${i + 1}`
    ),
  ];

  // Single query to find all existing slugs that match our potential slugs
  const existingSlugs = await db
    .select({ slug: organizations.slug })
    .from(organizations)
    .where(inArray(organizations.slug, potentialSlugs));

  const existingSlugSet = new Set(existingSlugs.map((r) => r.slug));

  // Return the first available slug
  for (const slug of potentialSlugs) {
    if (!existingSlugSet.has(slug)) {
      return slug;
    }
  }

  // Fallback: append random suffix (all 11 potential slugs are taken)
  return `${baseSlug}-${crypto.randomUUID().slice(0, 8)}`;
};

// Handle installation.created event - create organization and projects
const handleInstallationCreated = async (
  db: DbClient,
  installation: InstallationPayload["installation"],
  repositories: InstallationPayload["repositories"],
  sender: InstallationPayload["sender"]
): Promise<
  | { organizationId: string; slug: string }
  | { existing: true; id: string; slug: string; reactivated?: boolean }
> => {
  const { account } = installation;

  // Check by providerAccountId first (survives reinstalls - GitHub org/user ID is immutable)
  const existingByAccount = await db
    .select({
      id: organizations.id,
      slug: organizations.slug,
      deletedAt: organizations.deletedAt,
    })
    .from(organizations)
    .where(
      and(
        eq(organizations.provider, "github"),
        eq(organizations.providerAccountId, String(account.id))
      )
    )
    .limit(1);

  if (existingByAccount[0]) {
    const existing = existingByAccount[0];

    if (existing.deletedAt) {
      // Reactivate soft-deleted org with new installation
      await db
        .update(organizations)
        .set({
          deletedAt: null,
          providerInstallationId: String(installation.id),
          installerGithubId: String(sender.id),
          providerAccountLogin: account.login, // May have changed
          providerAvatarUrl: account.avatar_url ?? null,
          updatedAt: new Date(),
        })
        .where(eq(organizations.id, existing.id));

      // Reactivate soft-deleted projects for this org
      await db
        .update(projects)
        .set({
          removedAt: null,
          updatedAt: new Date(),
        })
        .where(
          and(
            eq(projects.organizationId, existing.id),
            isNotNull(projects.removedAt)
          )
        );

      console.log(
        `[installation] Reactivated soft-deleted organization: ${existing.slug} (${existing.id})`
      );

      // Create any new projects that weren't in the previous installation
      if (repositories && repositories.length > 0) {
        const projectValues = repositories.map((repo) => ({
          id: crypto.randomUUID(),
          organizationId: existing.id,
          handle: repo.name.toLowerCase(),
          providerRepoId: String(repo.id),
          providerRepoName: repo.name,
          providerRepoFullName: repo.full_name,
          isPrivate: repo.private,
        }));

        await db.insert(projects).values(projectValues).onConflictDoNothing();
      }

      // Try to auto-link the installer if they have an existing Detent account
      await autoLinkInstaller(db, existing.id, String(sender.id), sender.login);

      return {
        existing: true,
        id: existing.id,
        slug: existing.slug,
        reactivated: true,
      };
    }

    // Active org exists - idempotency: update installation ID and return
    await db
      .update(organizations)
      .set({
        providerInstallationId: String(installation.id),
        providerAccountLogin: account.login, // May have changed
        providerAvatarUrl: account.avatar_url ?? null,
        updatedAt: new Date(),
      })
      .where(eq(organizations.id, existing.id));

    console.log(
      `[installation] Organization already exists for account ${account.id}, updated installation: ${existing.slug}`
    );
    return { existing: true, id: existing.id, slug: existing.slug };
  }

  // Fallback: check by installation ID (handles edge case of duplicate webhooks)
  const existingByInstall = await db
    .select({ id: organizations.id, slug: organizations.slug })
    .from(organizations)
    .where(eq(organizations.providerInstallationId, String(installation.id)))
    .limit(1);

  if (existingByInstall[0]) {
    console.log(
      `[installation] Organization already exists for installation ${installation.id}: ${existingByInstall[0].slug}`
    );
    return {
      existing: true,
      id: existingByInstall[0].id,
      slug: existingByInstall[0].slug,
    };
  }

  // Create organization when app is installed
  const organizationId = crypto.randomUUID();
  // Use provider-prefixed slug format: gh/login or gl/login
  const baseSlug = createProviderSlug("github", account.login);
  const slug = await generateUniqueSlug(db, baseSlug);

  await db.insert(organizations).values({
    id: organizationId,
    name: account.login,
    slug,
    provider: "github",
    providerAccountId: String(account.id),
    providerAccountLogin: account.login,
    providerAccountType:
      account.type === "Organization" ? "organization" : "user",
    providerInstallationId: String(installation.id),
    providerAvatarUrl: account.avatar_url ?? null,
    // Track installer's GitHub ID (immutable) for owner role assignment
    installerGithubId: String(sender.id),
  });

  console.log(
    `[installation] Created organization: ${slug} (${organizationId})`
  );

  // Create projects for initial repositories
  if (repositories && repositories.length > 0) {
    const projectValues = repositories.map((repo) => ({
      id: crypto.randomUUID(),
      organizationId,
      handle: repo.name.toLowerCase(), // URL-friendly handle defaults to repo name
      providerRepoId: String(repo.id),
      providerRepoName: repo.name,
      providerRepoFullName: repo.full_name,
      isPrivate: repo.private,
    }));

    await db.insert(projects).values(projectValues).onConflictDoNothing();

    console.log(
      `[installation] Created ${repositories.length} projects for organization ${slug}`
    );
  }

  // Try to auto-link the installer if they have an existing Detent account
  await autoLinkInstaller(db, organizationId, String(sender.id), sender.login);

  return { organizationId, slug };
};

// Handle installation events (GitHub App installed/uninstalled)
const handleInstallationEvent = async (
  c: WebhookContext,
  payload: InstallationPayload
) => {
  const { action, installation, repositories } = payload;
  const { account } = installation;

  console.log(
    `[installation] ${action}: ${account.login} (${account.type}, installation ${installation.id})`
  );

  const { db, client } = await createDb(c.env);

  try {
    switch (action) {
      case "created": {
        const result = await handleInstallationCreated(
          db,
          installation,
          repositories,
          payload.sender
        );

        if ("existing" in result) {
          return c.json({
            message: result.reactivated
              ? "installation reactivated"
              : "installation already exists",
            organization_id: result.id,
            organization_slug: result.slug,
            account: account.login,
            reactivated: result.reactivated ?? false,
          });
        }

        return c.json({
          message: "installation created",
          organization_id: result.organizationId,
          organization_slug: result.slug,
          account: account.login,
          projects_created: repositories?.length ?? 0,
        });
      }

      case "deleted": {
        await db
          .update(organizations)
          .set({ deletedAt: new Date(), updatedAt: new Date() })
          .where(
            eq(organizations.providerInstallationId, String(installation.id))
          );

        console.log(
          `[installation] Soft-deleted organization for installation ${installation.id}`
        );

        return c.json({
          message: "installation deleted",
          account: account.login,
        });
      }

      case "suspend": {
        await db
          .update(organizations)
          .set({ suspendedAt: new Date(), updatedAt: new Date() })
          .where(
            eq(organizations.providerInstallationId, String(installation.id))
          );

        return c.json({
          message: "installation suspended",
          account: account.login,
        });
      }

      case "unsuspend": {
        await db
          .update(organizations)
          .set({ suspendedAt: null, updatedAt: new Date() })
          .where(
            eq(organizations.providerInstallationId, String(installation.id))
          );

        return c.json({
          message: "installation unsuspended",
          account: account.login,
        });
      }

      case "new_permissions_accepted": {
        // User accepted new permissions requested by the app
        // Update the organization's updatedAt to track this event
        await db
          .update(organizations)
          .set({ updatedAt: new Date() })
          .where(
            eq(organizations.providerInstallationId, String(installation.id))
          );

        console.log(
          `[installation] New permissions accepted for installation ${installation.id}`
        );

        return c.json({
          message: "permissions updated",
          account: account.login,
        });
      }

      default:
        return c.json({ message: "ignored", action });
    }
  } catch (error) {
    console.error("[installation] Error processing:", error);
    return c.json(
      {
        message: "installation error",
        error: error instanceof Error ? error.message : "Unknown error",
      },
      500
    );
  } finally {
    await client.end();
  }
};

// Handle installation_repositories events (repos added/removed from installation)
const handleInstallationRepositoriesEvent = async (
  c: WebhookContext,
  payload: InstallationRepositoriesPayload
) => {
  const { action, installation, repositories_added, repositories_removed } =
    payload;

  console.log(
    `[installation_repositories] ${action}: installation ${installation.id}, added=${repositories_added.length}, removed=${repositories_removed.length}`
  );

  const { db, client } = await createDb(c.env);

  try {
    // Find organization by installation ID
    const orgResult = await db
      .select({ id: organizations.id, slug: organizations.slug })
      .from(organizations)
      .where(eq(organizations.providerInstallationId, String(installation.id)))
      .limit(1);

    const org = orgResult[0];
    if (!org) {
      console.log(
        `[installation_repositories] Organization not found for installation ${installation.id}`
      );
      return c.json({
        message: "organization not found",
        installation_id: installation.id,
      });
    }

    // Handle added repositories
    if (repositories_added.length > 0) {
      const projectValues = repositories_added.map((repo) => ({
        id: crypto.randomUUID(),
        organizationId: org.id,
        handle: repo.name.toLowerCase(), // URL-friendly handle defaults to repo name
        providerRepoId: String(repo.id),
        providerRepoName: repo.name,
        providerRepoFullName: repo.full_name,
        isPrivate: repo.private,
      }));

      await db.insert(projects).values(projectValues).onConflictDoNothing();

      console.log(
        `[installation_repositories] Created ${repositories_added.length} projects for organization ${org.slug}`
      );
    }

    // Handle removed repositories (soft-delete) - batch update for performance
    if (repositories_removed.length > 0) {
      const repoIds = repositories_removed.map((repo) => String(repo.id));
      await db
        .update(projects)
        .set({ removedAt: new Date(), updatedAt: new Date() })
        .where(inArray(projects.providerRepoId, repoIds));

      console.log(
        `[installation_repositories] Soft-deleted ${repositories_removed.length} projects for organization ${org.slug}`
      );
    }

    return c.json({
      message: "installation_repositories processed",
      organization_id: org.id,
      organization_slug: org.slug,
      projects_added: repositories_added.length,
      projects_removed: repositories_removed.length,
    });
  } catch (error) {
    console.error("[installation_repositories] Error processing:", error);
    return c.json(
      {
        message: "installation_repositories error",
        error: error instanceof Error ? error.message : "Unknown error",
      },
      500
    );
  } finally {
    await client.end();
  }
};

// Handle repository events (renamed, transferred, visibility changed)
const handleRepositoryEvent = async (
  c: WebhookContext,
  payload: RepositoryPayload
) => {
  const { action, repository, installation } = payload;

  // Only process if we have an installation ID (app is installed)
  if (!installation?.id) {
    return c.json({ message: "ignored", reason: "no installation" });
  }

  console.log(
    `[repository] ${action}: ${repository.full_name} (repo ID: ${repository.id})`
  );

  const { db, client } = await createDb(c.env);

  try {
    // Find the project by provider repo ID
    const existingProject = await db
      .select({
        id: projects.id,
        handle: projects.handle,
        providerRepoName: projects.providerRepoName,
        providerRepoFullName: projects.providerRepoFullName,
        isPrivate: projects.isPrivate,
      })
      .from(projects)
      .where(eq(projects.providerRepoId, String(repository.id)))
      .limit(1);

    const project = existingProject[0];
    if (!project) {
      console.log(
        `[repository] Project not found for repo ID ${repository.id}, skipping`
      );
      return c.json({
        message: "project not found",
        repo_id: repository.id,
      });
    }

    switch (action) {
      case "renamed": {
        // Update repo name and full_name, but preserve custom handle
        await db
          .update(projects)
          .set({
            providerRepoName: repository.name,
            providerRepoFullName: repository.full_name,
            updatedAt: new Date(),
          })
          .where(eq(projects.id, project.id));

        console.log(
          `[repository] Updated project ${project.id}: ${project.providerRepoFullName} -> ${repository.full_name}`
        );

        return c.json({
          message: "repository renamed",
          project_id: project.id,
          old_name: project.providerRepoFullName,
          new_name: repository.full_name,
        });
      }

      case "privatized":
      case "publicized": {
        const isPrivate = action === "privatized";
        await db
          .update(projects)
          .set({
            isPrivate,
            updatedAt: new Date(),
          })
          .where(eq(projects.id, project.id));

        console.log(
          `[repository] Updated project ${project.id} visibility: private=${isPrivate}`
        );

        return c.json({
          message: `repository ${action}`,
          project_id: project.id,
          is_private: isPrivate,
        });
      }

      case "transferred": {
        // Repository was transferred to another owner
        // The project stays with the original org, but we update the full_name
        await db
          .update(projects)
          .set({
            providerRepoFullName: repository.full_name,
            updatedAt: new Date(),
          })
          .where(eq(projects.id, project.id));

        console.log(
          `[repository] Repository transferred, updated full_name to ${repository.full_name}`
        );

        return c.json({
          message: "repository transferred",
          project_id: project.id,
          new_full_name: repository.full_name,
        });
      }

      default:
        return c.json({ message: "ignored", action });
    }
  } catch (error) {
    console.error("[repository] Error processing:", error);
    return c.json(
      {
        message: "repository error",
        error: error instanceof Error ? error.message : "Unknown error",
      },
      500
    );
  } finally {
    await client.end();
  }
};

// Handle organization events (GitHub org renamed, etc.)
const handleOrganizationEvent = async (
  c: WebhookContext,
  payload: OrganizationPayload
) => {
  const { action, organization, changes, installation } = payload;

  // Only process if we have an installation ID (app is installed)
  if (!installation?.id) {
    return c.json({ message: "ignored", reason: "no installation" });
  }

  console.log(
    `[organization] ${action}: ${organization.login} (org ID: ${organization.id})`
  );

  // Only handle renamed action for now
  if (action !== "renamed") {
    return c.json({ message: "ignored", action });
  }

  const oldLogin = changes?.login?.from;
  if (!oldLogin) {
    console.log("[organization] No login change found in payload, skipping");
    return c.json({ message: "ignored", reason: "no login change" });
  }

  const { db, client } = await createDb(c.env);

  try {
    // Find the organization by provider account ID (immutable)
    const existingOrg = await db
      .select({
        id: organizations.id,
        slug: organizations.slug,
        providerAccountLogin: organizations.providerAccountLogin,
      })
      .from(organizations)
      .where(
        and(
          eq(organizations.provider, "github"),
          eq(organizations.providerAccountId, String(organization.id))
        )
      )
      .limit(1);

    const org = existingOrg[0];
    if (!org) {
      console.log(
        `[organization] Organization not found for GitHub org ID ${organization.id}, skipping`
      );
      return c.json({
        message: "organization not found",
        github_org_id: organization.id,
      });
    }

    // Update providerAccountLogin
    const updates: {
      providerAccountLogin: string;
      providerAvatarUrl: string | null;
      updatedAt: Date;
      slug?: string;
      name?: string;
    } = {
      providerAccountLogin: organization.login,
      providerAvatarUrl: organization.avatar_url ?? null,
      updatedAt: new Date(),
    };

    // Check if slug matches the provider pattern (gh/old-login)
    const oldProviderSlug = createProviderSlug("github", oldLogin);
    if (org.slug === oldProviderSlug) {
      // Update slug to match new login
      const newProviderSlug = createProviderSlug("github", organization.login);
      updates.slug = newProviderSlug;
      updates.name = organization.login;
    }

    await db
      .update(organizations)
      .set(updates)
      .where(eq(organizations.id, org.id));

    console.log(
      `[organization] Updated organization ${org.id}: login ${oldLogin} -> ${organization.login}${
        updates.slug ? `, slug ${org.slug} -> ${updates.slug}` : ""
      }`
    );

    return c.json({
      message: "organization renamed",
      organization_id: org.id,
      old_login: oldLogin,
      new_login: organization.login,
      old_slug: org.slug,
      new_slug: updates.slug ?? org.slug,
    });
  } catch (error) {
    console.error("[organization] Error processing:", error);
    return c.json(
      {
        message: "organization error",
        error: error instanceof Error ? error.message : "Unknown error",
      },
      500
    );
  } finally {
    await client.end();
  }
};

// Handle issue_comment events (@detent mentions)
const handleIssueCommentEvent = async (
  c: WebhookContext,
  payload: IssueCommentPayload
) => {
  const { action, comment, issue, repository, installation } = payload;

  // Only process new comments
  if (action !== "created") {
    return c.json({ message: "ignored", reason: "not created" });
  }

  // Only process PR comments (not issues)
  if (!issue.pull_request) {
    return c.json({ message: "ignored", reason: "not a pull request" });
  }

  // Check for @detent mention
  const body = comment.body.toLowerCase();
  if (!body.includes("@detent")) {
    return c.json({ message: "ignored", reason: "no @detent mention" });
  }

  console.log(
    `[issue_comment] @detent mentioned in ${repository.full_name}#${issue.number} by ${comment.user.login}`
  );

  // Parse command
  const command = parseDetentCommand(comment.body);

  // Get GitHub service
  const github = createGitHubService(c.env);

  try {
    // Get installation token
    const token = await github.getInstallationToken(installation.id);

    switch (command.type) {
      case "heal": {
        // Post acknowledgment
        await github.postComment(
          token,
          repository.owner.login,
          repository.name,
          issue.number,
          `🔧 **Detent** is analyzing the CI failures${command.dryRun ? " (dry run)" : ""}...`
        );

        // Healing flow will:
        // 1. Find latest failed workflow run
        // 2. Fetch and parse logs with @detent/parser
        // 3. Run healing loop with Claude via @detent/healing
        // 4. Push fix (if not dry run)
        // 5. Post results

        return c.json({
          message: "heal command received",
          repository: repository.full_name,
          issue: issue.number,
          dryRun: command.dryRun,
          status: "acknowledged",
        });
      }

      case "status": {
        // Future: Report current error status from stored analysis
        await github.postComment(
          token,
          repository.owner.login,
          repository.name,
          issue.number,
          "📊 **Detent** status check is not yet implemented."
        );
        return c.json({
          message: "status command received",
          status: "not_implemented",
        });
      }

      case "help": {
        await github.postComment(
          token,
          repository.owner.login,
          repository.name,
          issue.number,
          formatHelpMessage()
        );
        return c.json({ message: "help command received", status: "posted" });
      }

      default: {
        await github.postComment(
          token,
          repository.owner.login,
          repository.name,
          issue.number,
          `🤔 Unknown command. ${formatHelpMessage()}`
        );
        return c.json({ message: "unknown command", status: "posted" });
      }
    }
  } catch (error) {
    console.error("[issue_comment] Error processing:", error);
    return c.json(
      {
        message: "issue_comment error",
        error: error instanceof Error ? error.message : "Unknown error",
      },
      500
    );
  }
};

// Parse @detent commands from comment body
const parseDetentCommand = (body: string): DetentCommand => {
  const lower = body.toLowerCase();

  if (lower.includes("@detent heal")) {
    const dryRun = lower.includes("--dry") || lower.includes("--dry-run");
    return { type: "heal", dryRun };
  }

  if (lower.includes("@detent status")) {
    return { type: "status" };
  }

  if (lower.includes("@detent help")) {
    return { type: "help" };
  }

  return { type: "unknown" };
};

// Format failure comment for PR
// Future: Use parsed errors from @detent/parser to provide detailed error analysis
const formatFailureComment = (
  owner: string,
  repo: string,
  workflowName: string,
  runId: number,
  _logs: string
): string => {
  return `## ❌ CI Failed: ${workflowName}

[View workflow run](https://github.com/${owner}/${repo}/actions/runs/${runId})

<details>
<summary>🔍 Error Analysis</summary>

_Error parsing not yet implemented. Log extraction pending._

</details>

---
💡 **Tip:** Comment \`@detent heal\` to attempt automatic fixes.
`;
};

// Format help message
const formatHelpMessage = (): string => {
  return `**Available commands:**
- \`@detent heal\` - Analyze errors and attempt automatic fixes
- \`@detent heal --dry-run\` - Analyze without pushing changes
- \`@detent status\` - Show current error status
- \`@detent help\` - Show this message`;
};

export default app;
