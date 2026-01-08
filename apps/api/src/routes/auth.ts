/**
 * Auth routes
 *
 * Handles identity synchronization from WorkOS to update organization members
 * with GitHub identity information obtained during authentication.
 */

import { and, eq, inArray, isNull } from "drizzle-orm";
import { Hono } from "hono";
import { createDb } from "../db/client";
import { organizationMembers, organizations } from "../db/schema";
import { verifyGitHubMembership } from "../lib/github-membership";
import type { Env } from "../types/env";

// GitHub API types
interface GitHubOrg {
  id: number;
  login: string;
  avatar_url: string;
}

interface GitHubOrgMembership {
  role: "admin" | "member";
  state: "active" | "pending";
}

// Response type for github-orgs endpoint
interface GitHubOrgWithStatus {
  id: number;
  login: string;
  avatar_url: string;
  can_install: boolean;
  already_installed: boolean;
  detent_org_id?: string;
}

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

// Regex for validating numeric target_id parameter
const NUMERIC_REGEX = /^\d+$/;

/**
 * POST /sync-identity
 * Sync GitHub identity from WorkOS to all organization memberships for the authenticated user.
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

  // Find GitHub OAuth identity (with null check for malformed responses)
  const githubIdentity = identities.data?.find(
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

  // Update all organization memberships for this user with GitHub identity
  const { db, client } = await createDb(c.env);
  try {
    const updatedMembers = await db
      .update(organizationMembers)
      .set({
        providerUserId: githubUserId,
        providerUsername: githubUsername,
        providerLinkedAt: new Date(),
        updatedAt: new Date(),
      })
      .where(eq(organizationMembers.userId, auth.userId))
      .returning({
        organizationId: organizationMembers.organizationId,
        providerUsername: organizationMembers.providerUsername,
      });

    // Auto-link organizations where this user is the installer but has no membership
    const installerOrgs = await db.query.organizations.findMany({
      where: and(
        eq(organizations.installerGithubId, githubUserId),
        isNull(organizations.deletedAt)
      ),
    });

    // Batch fetch existing memberships for all installer orgs
    const orgIds = installerOrgs.map((org) => org.id);
    const existingMemberships =
      orgIds.length > 0
        ? await db.query.organizationMembers.findMany({
            where: and(
              eq(organizationMembers.userId, auth.userId),
              inArray(organizationMembers.organizationId, orgIds)
            ),
          })
        : [];

    const existingOrgIds = new Set(
      existingMemberships.map((m) => m.organizationId)
    );

    // Filter to orgs that need new memberships
    const orgsToLink = installerOrgs.filter(
      (org) => !existingOrgIds.has(org.id)
    );

    // Verify current GitHub membership before granting owner access
    const verifiedOrgs: typeof orgsToLink = [];
    if (githubUsername) {
      for (const org of orgsToLink) {
        if (!(org.providerInstallationId && org.providerAccountLogin)) {
          continue;
        }
        const membership = await verifyGitHubMembership(
          githubUsername,
          org.providerAccountLogin,
          org.providerInstallationId,
          c.env
        );
        if (membership.isMember) {
          verifiedOrgs.push(org);
        }
      }
    }

    // Batch insert (if any)
    if (verifiedOrgs.length > 0) {
      await db.insert(organizationMembers).values(
        verifiedOrgs.map((org) => ({
          id: crypto.randomUUID(),
          organizationId: org.id,
          userId: auth.userId,
          role: "owner" as const,
          providerUserId: githubUserId,
          providerUsername: githubUsername,
          providerLinkedAt: new Date(),
        }))
      );
    }

    const autoLinkedCount = verifiedOrgs.length;

    return c.json({
      user_id: auth.userId,
      email: user.email,
      first_name: user.first_name,
      last_name: user.last_name,
      github_synced: true,
      github_user_id: githubUserId,
      github_username: githubUsername,
      organizations_updated: updatedMembers.length,
      installer_orgs_linked: autoLinkedCount,
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
      db.query.organizationMembers.findFirst({
        where: eq(organizationMembers.userId, auth.userId),
      }),
    ]);

    if (!userResponse.ok) {
      console.error(
        `Failed to fetch user from WorkOS: ${userResponse.status} ${userResponse.statusText}`
      );
      return c.json({ error: "Failed to fetch user details" }, 500);
    }

    const user = (await userResponse.json()) as WorkOSUser;

    // If no organization membership found, also check WorkOS identities directly
    // This handles the case where a user authenticated via GitHub but has no organization yet
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
        const githubIdentity = identities.data?.find(
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

/**
 * GET /github-orgs
 * List GitHub organizations where the authenticated user can install the Detent GitHub App.
 * Returns org details with installation status and user's admin capability.
 */
app.get("/github-orgs", async (c) => {
  const auth = c.get("auth");

  // Fetch GitHub OAuth token from WorkOS Pipes API
  const tokenResponse = await fetch(
    "https://api.workos.com/data-integrations/github/token",
    {
      method: "POST",
      headers: {
        Authorization: `Bearer ${c.env.WORKOS_API_KEY}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        user_id: auth.userId,
      }),
    }
  );

  if (!tokenResponse.ok) {
    console.error(
      `Failed to fetch GitHub token from WorkOS: ${tokenResponse.status} ${tokenResponse.statusText}`
    );
    return c.json({ error: "Failed to fetch GitHub OAuth token" }, 500);
  }

  const tokenData = (await tokenResponse.json()) as {
    active: boolean;
    access_token?: { token: string; expires_at: string };
    error?: string;
  };

  if (!(tokenData.active && tokenData.access_token)) {
    let errorMessage = "GitHub OAuth token not available";
    if (tokenData.error === "not_installed") {
      errorMessage =
        "GitHub account not connected. Please authenticate with GitHub.";
    } else if (tokenData.error === "needs_reauthorization") {
      errorMessage =
        "GitHub authorization expired. Please re-authenticate with GitHub.";
    }
    return c.json({ error: errorMessage, code: tokenData.error }, 401);
  }

  const githubToken = tokenData.access_token.token;

  // Fetch user's GitHub organizations
  const orgsResponse = await fetch("https://api.github.com/user/orgs", {
    headers: {
      Authorization: `Bearer ${githubToken}`,
      Accept: "application/vnd.github.v3+json",
      "User-Agent": "Detent-API",
    },
  });

  if (!orgsResponse.ok) {
    console.error(
      `Failed to fetch GitHub orgs: ${orgsResponse.status} ${orgsResponse.statusText}`
    );
    return c.json({ error: "Failed to fetch GitHub organizations" }, 500);
  }

  const githubOrgs = (await orgsResponse.json()) as GitHubOrg[];

  if (githubOrgs.length === 0) {
    return c.json({ orgs: [] });
  }

  // Fetch membership details for each org in parallel to check admin status
  const membershipPromises = githubOrgs.map((org) =>
    fetch(`https://api.github.com/user/memberships/orgs/${org.login}`, {
      headers: {
        Authorization: `Bearer ${githubToken}`,
        Accept: "application/vnd.github.v3+json",
        "User-Agent": "Detent-API",
      },
    }).then(async (res) => {
      if (!res.ok) {
        return { org: org.login, membership: null };
      }
      const membership = (await res.json()) as GitHubOrgMembership;
      return { org: org.login, membership };
    })
  );

  const memberships = await Promise.all(membershipPromises);
  const membershipMap = new Map(memberships.map((m) => [m.org, m.membership]));

  // Query our database to find which orgs are already installed
  const { db, client } = await createDb(c.env);
  try {
    const githubOrgIds = githubOrgs.map((org) => String(org.id));
    const installedOrgs = await db.query.organizations.findMany({
      where: (orgs, { and, eq, inArray, isNull }) =>
        and(
          eq(orgs.provider, "github"),
          inArray(orgs.providerAccountId, githubOrgIds),
          isNull(orgs.deletedAt)
        ),
    });

    const installedOrgMap = new Map(
      installedOrgs.map((org) => [org.providerAccountId, org])
    );

    // Build response
    const orgsWithStatus: GitHubOrgWithStatus[] = githubOrgs.map((org) => {
      const membership = membershipMap.get(org.login);
      const installedOrg = installedOrgMap.get(String(org.id));

      return {
        id: org.id,
        login: org.login,
        avatar_url: org.avatar_url,
        can_install:
          membership?.role === "admin" && membership?.state === "active",
        already_installed: Boolean(installedOrg),
        ...(installedOrg && { detent_org_id: installedOrg.id }),
      };
    });

    return c.json({ orgs: orgsWithStatus });
  } finally {
    await client.end();
  }
});

/**
 * GET /install-url
 * Generate a GitHub App installation URL with a pre-selected target organization
 */
app.get("/install-url", (c) => {
  const targetId = c.req.query("target_id");

  if (!targetId) {
    return c.json({ error: "target_id query parameter is required" }, 400);
  }

  if (!NUMERIC_REGEX.test(targetId)) {
    return c.json({ error: "target_id must be a numeric value" }, 400);
  }

  const appName = "detent";
  const url = `https://github.com/apps/${appName}/installations/new?target_id=${targetId}`;

  return c.json({ url });
});

/**
 * GET /organizations
 * List organizations the user is a member of
 */
app.get("/organizations", async (c) => {
  const auth = c.get("auth");

  const { db, client } = await createDb(c.env);
  try {
    const memberships = await db.query.organizationMembers.findMany({
      where: eq(organizationMembers.userId, auth.userId),
      with: { organization: true },
    });

    return c.json({
      organizations: memberships.map((m) => ({
        organization_id: m.organizationId,
        organization_name: m.organization.name,
        organization_slug: m.organization.slug,
        github_org: m.organization.providerAccountLogin,
        role: m.role,
        github_linked: Boolean(m.providerUserId),
        github_username: m.providerUsername,
      })),
    });
  } finally {
    await client.end();
  }
});

export default app;
