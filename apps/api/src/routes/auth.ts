/**
 * Auth routes
 *
 * Handles identity synchronization from WorkOS to update team members
 * with GitHub identity information obtained during authentication.
 */

import { eq } from "drizzle-orm";
import { Hono } from "hono";
import { createDb } from "../db/client";
import { teamMembers } from "../db/schema";
import type { Env } from "../types/env";

// WorkOS identity from /user_management/users/:id/identities
interface WorkOSIdentity {
  idp_id: string;
  type: "OAuth";
  provider: string;
}

// Response from WorkOS identities endpoint
interface WorkOSIdentitiesResponse {
  data: WorkOSIdentity[];
}

// Response from WorkOS get user endpoint
interface WorkOSUser {
  id: string;
  email: string;
  first_name?: string;
  last_name?: string;
  profile_picture_url?: string;
}

const app = new Hono<{ Bindings: Env }>();

/**
 * POST /sync-identity
 * Sync GitHub identity from WorkOS to all team memberships for the authenticated user.
 * This is called after successful device code authentication to capture GitHub identity
 * if the user authenticated via GitHub OAuth through WorkOS.
 */
app.post("/sync-identity", async (c) => {
  const auth = c.get("auth");

  // Fetch user details and identities from WorkOS in parallel
  const [userResponse, identitiesResponse] = await Promise.all([
    fetch(`https://api.workos.com/user_management/users/${auth.userId}`, {
      headers: {
        Authorization: `Bearer ${c.env.WORKOS_API_KEY}`,
      },
    }),
    fetch(
      `https://api.workos.com/user_management/users/${auth.userId}/identities`,
      {
        headers: {
          Authorization: `Bearer ${c.env.WORKOS_API_KEY}`,
        },
      }
    ),
  ]);

  if (!userResponse.ok) {
    console.error(
      `Failed to fetch user from WorkOS: ${userResponse.status} ${userResponse.statusText}`
    );
    return c.json({ error: "Failed to fetch user details" }, 500);
  }

  const user = (await userResponse.json()) as WorkOSUser;

  if (!identitiesResponse.ok) {
    console.error(
      `Failed to fetch identities from WorkOS: ${identitiesResponse.status} ${identitiesResponse.statusText}`
    );
    // Return user info without GitHub identity - this is not a fatal error
    return c.json({
      user_id: auth.userId,
      email: user.email,
      first_name: user.first_name,
      last_name: user.last_name,
      github_synced: false,
      github_username: null,
    });
  }

  const identities =
    (await identitiesResponse.json()) as WorkOSIdentitiesResponse;

  // Find GitHub OAuth identity
  const githubIdentity = identities.data.find(
    (identity) => identity.provider === "GitHubOAuth"
  );

  if (!githubIdentity) {
    // No GitHub identity linked - return user info without GitHub
    return c.json({
      user_id: auth.userId,
      email: user.email,
      first_name: user.first_name,
      last_name: user.last_name,
      github_synced: false,
      github_username: null,
    });
  }

  // We have the GitHub idp_id (numeric GitHub user ID)
  // To get the username, we need to query GitHub API
  const githubUserId = githubIdentity.idp_id;
  let githubUsername: string | null = null;

  try {
    const githubResponse = await fetch(
      `https://api.github.com/user/${githubUserId}`,
      {
        headers: {
          Accept: "application/vnd.github.v3+json",
          "User-Agent": "Detent-API",
        },
      }
    );

    if (githubResponse.ok) {
      const githubUser = (await githubResponse.json()) as { login: string };
      githubUsername = githubUser.login;
    }
  } catch {
    // GitHub API call failed, continue without username
    console.error("Failed to fetch GitHub username for user:", githubUserId);
  }

  // Update all team memberships for this user with GitHub identity
  const { db, client } = await createDb(c.env);
  try {
    const updatedMembers = await db
      .update(teamMembers)
      .set({
        providerUserId: githubUserId,
        providerUsername: githubUsername,
        providerLinkedAt: new Date(),
        updatedAt: new Date(),
      })
      .where(eq(teamMembers.userId, auth.userId))
      .returning({
        teamId: teamMembers.teamId,
        providerUsername: teamMembers.providerUsername,
      });

    return c.json({
      user_id: auth.userId,
      email: user.email,
      first_name: user.first_name,
      last_name: user.last_name,
      github_synced: true,
      github_user_id: githubUserId,
      github_username: githubUsername,
      teams_updated: updatedMembers.length,
    });
  } finally {
    await client.end();
  }
});

/**
 * GET /me
 * Get the current user's identity information including GitHub link status
 */
app.get("/me", async (c) => {
  const auth = c.get("auth");

  // Fetch user details from WorkOS and check DB membership in parallel
  const { db, client } = await createDb(c.env);
  try {
    const [userResponse, membership] = await Promise.all([
      fetch(`https://api.workos.com/user_management/users/${auth.userId}`, {
        headers: {
          Authorization: `Bearer ${c.env.WORKOS_API_KEY}`,
        },
      }),
      db.query.teamMembers.findFirst({
        where: eq(teamMembers.userId, auth.userId),
      }),
    ]);

    if (!userResponse.ok) {
      console.error(
        `Failed to fetch user from WorkOS: ${userResponse.status} ${userResponse.statusText}`
      );
      return c.json({ error: "Failed to fetch user details" }, 500);
    }

    const user = (await userResponse.json()) as WorkOSUser;

    // If no team membership found, also check WorkOS identities directly
    // This handles the case where a user authenticated via GitHub but has no team yet
    let githubUserId: string | null = membership?.providerUserId ?? null;
    const githubUsername: string | null = membership?.providerUsername ?? null;
    let githubLinked = Boolean(githubUserId);

    if (!githubLinked) {
      // Check WorkOS identities for GitHub OAuth
      const identitiesResponse = await fetch(
        `https://api.workos.com/user_management/users/${auth.userId}/identities`,
        {
          headers: {
            Authorization: `Bearer ${c.env.WORKOS_API_KEY}`,
          },
        }
      );

      if (identitiesResponse.ok) {
        const identities =
          (await identitiesResponse.json()) as WorkOSIdentitiesResponse;
        const githubIdentity = identities.data.find(
          (identity) => identity.provider === "GitHubOAuth"
        );

        if (githubIdentity) {
          githubUserId = githubIdentity.idp_id;
          githubLinked = true;
          // Note: username not available from WorkOS identity, would need GitHub API call
        }
      }
    }

    return c.json({
      user_id: auth.userId,
      email: user.email,
      first_name: user.first_name,
      last_name: user.last_name,
      github_linked: githubLinked,
      github_user_id: githubUserId,
      github_username: githubUsername,
    });
  } finally {
    await client.end();
  }
});

export default app;
