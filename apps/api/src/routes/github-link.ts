/**
 * GitHub account linking routes
 *
 * Enables WorkOS users to link their GitHub identity via WorkOS GitHub OAuth.
 * This creates the mapping between WorkOS user IDs and GitHub accounts.
 * Implements PKCE (Proof Key for Code Exchange) per OAuth 2.1.
 */

import { and, eq } from "drizzle-orm";
import { Hono } from "hono";
import { jwtVerify, SignJWT } from "jose";
import { createDb } from "../db/client";
import { teamMembers } from "../db/schema";
import type { Env } from "../types/env";

// State token payload for CSRF protection and PKCE
interface StatePayload {
  userId: string;
  teamId: string;
  redirectUri: string;
  codeChallenge: string;
}

// WorkOS SSO profile response
interface WorkOSProfile {
  id: string;
  idp_id: string;
  email: string;
  first_name?: string;
  last_name?: string;
  raw_attributes: {
    id: number;
    login: string;
    avatar_url?: string;
  };
}

interface WorkOSSSOResponse {
  profile: WorkOSProfile;
  access_token?: string;
}

const app = new Hono<{ Bindings: Env }>();

// Regex for validating localhost redirect URIs (at module level for performance)
const LOCALHOST_URI_REGEX = /^http:\/\/(localhost|127\.0\.0\.1)(:\d+)?\//;

// Sign state token as JWT for CSRF protection and integrity
const encodeState = async (
  payload: StatePayload,
  secret: string
): Promise<string> => {
  const secretKey = new TextEncoder().encode(secret);
  const token = await new SignJWT({ ...payload })
    .setProtectedHeader({ alg: "HS256" })
    .setIssuedAt()
    .setExpirationTime("10m")
    .sign(secretKey);
  return token;
};

const decodeState = async (
  state: string,
  secret: string
): Promise<StatePayload> => {
  const secretKey = new TextEncoder().encode(secret);
  const { payload } = await jwtVerify(state, secretKey);
  return {
    userId: payload.userId as string,
    teamId: payload.teamId as string,
    redirectUri: payload.redirectUri as string,
    codeChallenge: payload.codeChallenge as string,
  };
};

/**
 * GET /authorize
 * Generate WorkOS authorization URL for GitHub OAuth with PKCE
 */
app.get("/authorize", async (c) => {
  const auth = c.get("auth");
  const teamId = c.req.query("team_id");
  const redirectUri = c.req.query("redirect_uri");
  const codeChallenge = c.req.query("code_challenge");

  if (!teamId) {
    return c.json({ error: "team_id is required" }, 400);
  }

  if (!redirectUri) {
    return c.json({ error: "redirect_uri is required" }, 400);
  }

  if (!codeChallenge) {
    return c.json({ error: "code_challenge is required" }, 400);
  }

  // Validate redirect_uri against whitelist (localhost for CLI, plus any configured URIs)
  const isLocalhostUri = LOCALHOST_URI_REGEX.test(redirectUri);
  const allowedUris = c.env.ALLOWED_REDIRECT_URIS?.split(",") ?? [];
  if (!(isLocalhostUri || allowedUris.includes(redirectUri))) {
    return c.json({ error: "Invalid redirect_uri" }, 400);
  }

  // Verify user is a member of this team
  const { db, client } = await createDb(c.env);
  try {
    const member = await db.query.teamMembers.findFirst({
      where: and(
        eq(teamMembers.userId, auth.userId),
        eq(teamMembers.teamId, teamId)
      ),
    });

    if (!member) {
      return c.json({ error: "Not a member of this team" }, 403);
    }

    // Generate signed state token (expires in 10 minutes)
    // Include redirectUri and codeChallenge in state for OAuth security
    const state = await encodeState(
      { userId: auth.userId, teamId, redirectUri, codeChallenge },
      c.env.WORKOS_API_KEY
    );

    // Build WorkOS authorization URL with PKCE
    const authUrl = new URL("https://api.workos.com/sso/authorize");
    authUrl.searchParams.set("client_id", c.env.WORKOS_CLIENT_ID);
    authUrl.searchParams.set("redirect_uri", redirectUri);
    authUrl.searchParams.set("response_type", "code");
    authUrl.searchParams.set("provider", "GitHubOAuth");
    authUrl.searchParams.set("provider_scopes", "read:user,read:org");
    authUrl.searchParams.set("state", state);
    // PKCE: code_challenge and method (S256 = SHA256)
    authUrl.searchParams.set("code_challenge", codeChallenge);
    authUrl.searchParams.set("code_challenge_method", "S256");

    return c.json({
      authorization_url: authUrl.toString(),
      state,
    });
  } finally {
    await client.end();
  }
});

/**
 * POST /callback
 * Exchange authorization code for GitHub identity and link account (with PKCE)
 */
app.post("/callback", async (c) => {
  const auth = c.get("auth");
  const body = await c.req.json<{
    code: string;
    state: string;
    team_id: string;
    code_verifier: string;
  }>();

  const { code, state, team_id: teamId, code_verifier: codeVerifier } = body;

  if (!(code && state && teamId && codeVerifier)) {
    return c.json(
      { error: "code, state, team_id, and code_verifier are required" },
      400
    );
  }

  // Verify and decode signed state token
  let statePayload: StatePayload;
  try {
    statePayload = await decodeState(state, c.env.WORKOS_API_KEY);
  } catch (error) {
    // JWT verification handles expiration and signature validation
    const message = error instanceof Error ? error.message : "Invalid token";
    if (message.includes("expired")) {
      return c.json({ error: "State token expired" }, 400);
    }
    return c.json({ error: "Invalid state token" }, 400);
  }

  // Verify state matches request
  if (statePayload.userId !== auth.userId || statePayload.teamId !== teamId) {
    return c.json({ error: "State mismatch" }, 400);
  }

  // Exchange code for tokens via WorkOS (with PKCE code_verifier)
  // redirect_uri must match the one used in the authorization request
  const tokenResponse = await fetch("https://api.workos.com/sso/token", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      client_id: c.env.WORKOS_CLIENT_ID,
      client_secret: c.env.WORKOS_API_KEY,
      grant_type: "authorization_code",
      code,
      redirect_uri: statePayload.redirectUri,
      code_verifier: codeVerifier,
    }),
  });

  if (!tokenResponse.ok) {
    const errorText = await tokenResponse.text();
    console.error("WorkOS token exchange failed:", errorText);

    // Parse error for more specific messaging
    let errorMessage = "Failed to exchange authorization code";
    try {
      const errorJson = JSON.parse(errorText) as {
        error?: string;
        error_description?: string;
      };
      if (errorJson.error === "invalid_grant") {
        errorMessage = "Authorization code expired or already used";
      } else if (errorJson.error_description) {
        errorMessage = errorJson.error_description;
      }
    } catch {
      // Keep default error message
    }

    return c.json({ error: errorMessage }, 500);
  }

  const ssoResponse = (await tokenResponse.json()) as WorkOSSSOResponse;
  const { profile } = ssoResponse;

  // Extract GitHub identity from profile
  const providerUserId = String(profile.raw_attributes?.id ?? profile.idp_id);
  const providerUsername = profile.raw_attributes?.login;

  if (!(providerUserId && providerUsername)) {
    return c.json({ error: "Failed to extract GitHub identity" }, 500);
  }

  // Update team member with provider identity
  const { db, client } = await createDb(c.env);
  try {
    await db
      .update(teamMembers)
      .set({
        providerUserId,
        providerUsername,
        providerLinkedAt: new Date(),
        updatedAt: new Date(),
      })
      .where(
        and(eq(teamMembers.userId, auth.userId), eq(teamMembers.teamId, teamId))
      );

    return c.json({
      success: true,
      github_user_id: providerUserId,
      github_username: providerUsername,
    });
  } finally {
    await client.end();
  }
});

/**
 * GET /status
 * Get GitHub link status for a user's team membership
 */
app.get("/status", async (c) => {
  const auth = c.get("auth");
  const teamId = c.req.query("team_id");

  if (!teamId) {
    return c.json({ error: "team_id is required" }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    const member = await db.query.teamMembers.findFirst({
      where: and(
        eq(teamMembers.userId, auth.userId),
        eq(teamMembers.teamId, teamId)
      ),
      with: { team: true },
    });

    if (!member) {
      return c.json({ error: "Not a member of this team" }, 404);
    }

    return c.json({
      team_id: member.teamId,
      team_name: member.team.name,
      team_slug: member.team.slug,
      github_org: member.team.providerAccountLogin,
      github_linked: Boolean(member.providerUserId),
      github_user_id: member.providerUserId,
      github_username: member.providerUsername,
      github_linked_at: member.providerLinkedAt?.toISOString() ?? null,
    });
  } finally {
    await client.end();
  }
});

/**
 * GET /teams
 * List teams the user is a member of (for team selection in CLI)
 */
app.get("/teams", async (c) => {
  const auth = c.get("auth");

  const { db, client } = await createDb(c.env);
  try {
    const memberships = await db.query.teamMembers.findMany({
      where: eq(teamMembers.userId, auth.userId),
      with: { team: true },
    });

    return c.json({
      teams: memberships.map((m) => ({
        team_id: m.teamId,
        team_name: m.team.name,
        team_slug: m.team.slug,
        github_org: m.team.providerAccountLogin,
        role: m.role,
        github_linked: Boolean(m.providerUserId),
        github_username: m.providerUsername,
      })),
    });
  } finally {
    await client.end();
  }
});

/**
 * GET /app-status
 * Get GitHub App installation status for a team
 */
app.get("/app-status", async (c) => {
  const auth = c.get("auth");
  const teamId = c.req.query("team_id");

  if (!teamId) {
    return c.json({ error: "team_id is required" }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    const member = await db.query.teamMembers.findFirst({
      where: and(
        eq(teamMembers.userId, auth.userId),
        eq(teamMembers.teamId, teamId)
      ),
      with: { team: true },
    });

    if (!member) {
      return c.json({ error: "Not a member of this team" }, 404);
    }

    const { team } = member;
    const installed = Boolean(team.providerInstallationId);

    return c.json({
      team_id: team.id,
      team_name: team.name,
      team_slug: team.slug,
      github_org: team.providerAccountLogin,
      app_installed: installed,
      installation_id: team.providerInstallationId ?? null,
      installed_at: team.createdAt?.toISOString() ?? null,
      suspended_at: team.suspendedAt?.toISOString() ?? null,
    });
  } finally {
    await client.end();
  }
});

/**
 * POST /unlink
 * Unlink GitHub account from team membership
 */
app.post("/unlink", async (c) => {
  const auth = c.get("auth");
  const body = await c.req.json<{ team_id: string }>();
  const { team_id: teamId } = body;

  if (!teamId) {
    return c.json({ error: "team_id is required" }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Verify user is a member and has a linked account
    const member = await db.query.teamMembers.findFirst({
      where: and(
        eq(teamMembers.userId, auth.userId),
        eq(teamMembers.teamId, teamId)
      ),
    });

    if (!member) {
      return c.json({ error: "Not a member of this team" }, 404);
    }

    if (!member.providerUserId) {
      return c.json({ error: "No GitHub account linked" }, 400);
    }

    // Clear the provider identity fields
    await db
      .update(teamMembers)
      .set({
        providerUserId: null,
        providerUsername: null,
        providerLinkedAt: null,
        updatedAt: new Date(),
      })
      .where(
        and(eq(teamMembers.userId, auth.userId), eq(teamMembers.teamId, teamId))
      );

    return c.json({ success: true });
  } finally {
    await client.end();
  }
});

export default app;
