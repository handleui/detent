import type { Context } from "hono";
import { Hono } from "hono";
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

interface DetentCommand {
  type: "heal" | "status" | "help" | "unknown";
  dryRun?: boolean;
}

// Variables stored in context by middleware
interface WebhookVariables {
  webhookPayload: WorkflowRunPayload | IssueCommentPayload | PingPayload;
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

    // 2. Check if there's an associated PR
    const prNumber =
      workflow_run.pull_requests[0]?.number ??
      (await github.getPullRequestForRun(
        token,
        repository.owner.login,
        repository.name,
        workflow_run.id
      ));

    if (!prNumber) {
      console.log("[workflow_run] No associated PR found, skipping comment");
      return c.json({
        message: "workflow_run processed",
        repository: repository.full_name,
        runId: workflow_run.id,
        status: "no_pr",
      });
    }

    // 3. Fetch workflow logs
    const logs = await github.fetchWorkflowLogs(
      token,
      repository.owner.login,
      repository.name,
      workflow_run.id
    );

    // TODO: Parse errors with @detent/parser
    // const errors = parseWorkflowLogs(logs);

    // 4. Post summary comment on PR
    const commentBody = formatFailureComment(
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

        // TODO: Implement healing flow
        // 1. Find latest failed workflow run
        // 2. Fetch and parse logs
        // 3. Run healing loop with Claude
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
        // TODO: Report current error status
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
const formatFailureComment = (
  workflowName: string,
  runId: number,
  _logs: string
): string => {
  // TODO: Actually parse and format errors from logs
  return `## ❌ CI Failed: ${workflowName}

[View workflow run](https://github.com/actions/runs/${runId})

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
