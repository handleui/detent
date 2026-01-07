import { eq, inArray } from "drizzle-orm";
import type { Context } from "hono";
import { Hono } from "hono";
import { createDb } from "../db/client";
import { organizations, projects } from "../db/schema";
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
    | InstallationRepositoriesPayload;
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

const generateUniqueSlug = async (
  db: DbClient,
  baseSlug: string
): Promise<string> => {
  let slug = baseSlug;
  let slugAttempt = 0;
  const maxSlugAttempts = 10;

  while (slugAttempt < maxSlugAttempts) {
    const slugConflict = await db
      .select({ id: organizations.id })
      .from(organizations)
      .where(eq(organizations.slug, slug))
      .limit(1);

    if (slugConflict.length === 0) {
      return slug;
    }

    slugAttempt++;
    slug = `${baseSlug}-${slugAttempt}`;
  }

  // Fallback: append random suffix
  return `${baseSlug}-${crypto.randomUUID().slice(0, 8)}`;
};

// Handle installation.created event - create organization and projects
const handleInstallationCreated = async (
  db: DbClient,
  installation: InstallationPayload["installation"],
  repositories: InstallationPayload["repositories"]
): Promise<
  | { organizationId: string; slug: string }
  | { existing: true; id: string; slug: string }
> => {
  const { account } = installation;

  // Idempotency check: if organization already exists for this installation, return it
  const existingOrg = await db
    .select({ id: organizations.id, slug: organizations.slug })
    .from(organizations)
    .where(eq(organizations.providerInstallationId, String(installation.id)))
    .limit(1);

  const existing = existingOrg[0];
  if (existing) {
    console.log(
      `[installation] Organization already exists for installation ${installation.id}: ${existing.slug}`
    );
    return { existing: true, id: existing.id, slug: existing.slug };
  }

  // Create organization when app is installed
  const organizationId = crypto.randomUUID();
  const baseSlug = account.login.toLowerCase().replace(/[^a-z0-9-]/g, "-");
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
  });

  console.log(
    `[installation] Created organization: ${slug} (${organizationId})`
  );

  // Create projects for initial repositories
  if (repositories && repositories.length > 0) {
    const projectValues = repositories.map((repo) => ({
      id: crypto.randomUUID(),
      organizationId,
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
          repositories
        );

        if ("existing" in result) {
          return c.json({
            message: "installation already exists",
            organization_id: result.id,
            organization_slug: result.slug,
            account: account.login,
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
