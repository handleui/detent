import type { Context } from "hono";
import { Hono } from "hono";
import { webhookSignatureMiddleware } from "../middleware/webhook-signature";
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
const handleWorkflowRunEvent = (
  c: WebhookContext,
  payload: WorkflowRunPayload
) => {
  const { action, workflow_run, repository } = payload;

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

  // TODO: Implement actual handling
  // 1. Get installation token
  // 2. Fetch workflow logs
  // 3. Parse errors with @detent/parser
  // 4. Post comment on PR (if associated)

  return c.json({
    message: "workflow_run received",
    repository: repository.full_name,
    workflow: workflow_run.name,
    runId: workflow_run.id,
    conclusion: workflow_run.conclusion,
  });
};

// Handle issue_comment events (@detent mentions)
const handleIssueCommentEvent = (
  c: WebhookContext,
  payload: IssueCommentPayload
) => {
  const { action, comment, issue, repository } = payload;

  // Only process new comments
  if (action !== "created") {
    return c.json({ message: "ignored", reason: "not created" });
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

  // TODO: Implement actual handling
  // 1. Get installation token
  // 2. Based on command:
  //    - "heal": Run healing loop, push fix
  //    - "status": Report current errors
  //    - "help": Post help message

  return c.json({
    message: "issue_comment received",
    repository: repository.full_name,
    issue: issue.number,
    command,
  });
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

export default app;
