/**
 * Invitation API routes
 *
 * Handles organization invitation management:
 * - Creating and sending invitations (org-scoped, owner/admin only)
 * - Listing pending invitations (org-scoped, owner/admin only)
 * - Revoking invitations (org-scoped, owner/admin only)
 * - Getting invitation details (public, for acceptance UI)
 * - Accepting invitations (authenticated, requires GitHub link)
 */

import { and, eq } from "drizzle-orm";
import { Hono } from "hono";
import { createDb } from "../db/client";
import { invitations, organizationMembers } from "../db/schema";
import { generateSecureToken } from "../lib/crypto";
import { getVerifiedGitHubIdentity } from "../lib/github-identity";
import { validateEmail } from "../lib/validation";
import {
  githubOrgAccessMiddleware,
  type OrgAccessContext,
  requireRole,
} from "../middleware/github-org-access";
import { createEmailService } from "../services/email";
import type { Env } from "../types/env";

// 7 days expiration
const INVITATION_EXPIRY_DAYS = 7;

// ============================================================================
// Org-scoped invitation routes (mounted at /orgs/:orgId/invitations)
// ============================================================================

export const orgInvitationsRoutes = new Hono<{ Bindings: Env }>();

/**
 * POST /orgs/:orgId/invitations - Create invitation
 * Owner/admin only
 */
orgInvitationsRoutes.post(
  "/",
  githubOrgAccessMiddleware,
  requireRole("owner", "admin"),
  async (c) => {
    const orgAccess = c.get("orgAccess") as OrgAccessContext;
    const { organization, githubIdentity } = orgAccess;
    const auth = c.get("auth");

    const body = await c.req.json<{ email: string; role?: string }>();
    const { email: rawEmail, role = "member" } = body;

    // Validate email
    const emailValidation = validateEmail(rawEmail);
    if (!emailValidation.valid) {
      return c.json({ error: emailValidation.error }, 400);
    }
    const email = rawEmail.trim().toLowerCase();

    // Validate role (only admin or member can be invited, not owner)
    if (!["admin", "member"].includes(role)) {
      return c.json(
        {
          error: "Role must be 'admin' or 'member'. Owners cannot be invited.",
        },
        400
      );
    }

    const { db, client } = await createDb(c.env);
    try {
      // Check for existing pending invitation
      const existingInvitation = await db.query.invitations.findFirst({
        where: and(
          eq(invitations.organizationId, organization.id),
          eq(invitations.email, email),
          eq(invitations.status, "pending")
        ),
      });

      if (existingInvitation) {
        return c.json(
          { error: "A pending invitation already exists for this email" },
          409
        );
      }

      // Generate secure token and expiration
      const token = generateSecureToken();
      const expiresAt = new Date();
      expiresAt.setDate(expiresAt.getDate() + INVITATION_EXPIRY_DAYS);

      // Create invitation
      const invitationId = crypto.randomUUID();
      await db.insert(invitations).values({
        id: invitationId,
        organizationId: organization.id,
        email,
        role: role as "admin" | "member",
        token,
        status: "pending",
        expiresAt,
        invitedBy: auth.userId,
      });

      // Send invitation email
      const acceptUrl = `${c.env.APP_BASE_URL}/invitations/${token}`;
      const emailService = createEmailService(c.env);

      try {
        await emailService.sendInvitationEmail({
          to: email,
          organizationName: organization.name,
          inviterName: githubIdentity.username,
          role,
          acceptUrl,
          expiresAt,
        });
      } catch (emailError) {
        // Rollback invitation if email fails
        await db.delete(invitations).where(eq(invitations.id, invitationId));
        console.error("[invitations] Email send failed:", emailError);
        return c.json({ error: "Failed to send invitation email" }, 500);
      }

      console.log(
        `[invitations] Created invitation for ${email} to ${organization.slug} as ${role}`
      );

      return c.json(
        {
          id: invitationId,
          email,
          role,
          expires_at: expiresAt.toISOString(),
          created_at: new Date().toISOString(),
        },
        201
      );
    } finally {
      await client.end();
    }
  }
);

/**
 * GET /orgs/:orgId/invitations - List pending invitations
 * Owner/admin only
 */
orgInvitationsRoutes.get(
  "/",
  githubOrgAccessMiddleware,
  requireRole("owner", "admin"),
  async (c) => {
    const orgAccess = c.get("orgAccess") as OrgAccessContext;
    const { organization } = orgAccess;

    const { db, client } = await createDb(c.env);
    try {
      const pendingInvitations = await db.query.invitations.findMany({
        where: and(
          eq(invitations.organizationId, organization.id),
          eq(invitations.status, "pending")
        ),
        orderBy: (inv, { desc }) => [desc(inv.createdAt)],
      });

      // Filter out expired invitations (they're still "pending" in DB but functionally expired)
      const now = new Date();
      const activeInvitations = pendingInvitations.filter(
        (inv) => inv.expiresAt > now
      );

      return c.json({
        invitations: activeInvitations.map((inv) => ({
          id: inv.id,
          email: inv.email,
          role: inv.role,
          invited_by: inv.invitedBy,
          expires_at: inv.expiresAt.toISOString(),
          created_at: inv.createdAt.toISOString(),
        })),
      });
    } finally {
      await client.end();
    }
  }
);

/**
 * DELETE /orgs/:orgId/invitations/:invitationId - Revoke invitation
 * Owner/admin only
 */
orgInvitationsRoutes.delete(
  "/:invitationId",
  githubOrgAccessMiddleware,
  requireRole("owner", "admin"),
  async (c) => {
    const orgAccess = c.get("orgAccess") as OrgAccessContext;
    const { organization } = orgAccess;
    const auth = c.get("auth");
    const invitationId = c.req.param("invitationId");

    const { db, client } = await createDb(c.env);
    try {
      // Find the invitation
      const invitation = await db.query.invitations.findFirst({
        where: and(
          eq(invitations.id, invitationId),
          eq(invitations.organizationId, organization.id)
        ),
      });

      if (!invitation) {
        return c.json({ error: "Invitation not found" }, 404);
      }

      if (invitation.status !== "pending") {
        return c.json(
          {
            error: `Cannot revoke invitation with status '${invitation.status}'`,
          },
          400
        );
      }

      // Mark as revoked
      await db
        .update(invitations)
        .set({
          status: "revoked",
          revokedAt: new Date(),
          revokedBy: auth.userId,
          updatedAt: new Date(),
        })
        .where(eq(invitations.id, invitationId));

      console.log(
        `[invitations] Revoked invitation ${invitationId} for ${invitation.email}`
      );

      return c.json({ success: true });
    } finally {
      await client.end();
    }
  }
);

// ============================================================================
// Standalone invitation routes (mounted at /invitations)
// ============================================================================

export const invitationRoutes = new Hono<{ Bindings: Env }>();

/**
 * GET /invitations/:token - Get invitation details (public)
 * Used by the acceptance UI to display invitation info
 */
invitationRoutes.get("/:token", async (c) => {
  const token = c.req.param("token");

  const { db, client } = await createDb(c.env);
  try {
    const invitation = await db.query.invitations.findFirst({
      where: eq(invitations.token, token),
      with: { organization: true },
    });

    if (!invitation) {
      return c.json({ error: "Invitation not found" }, 404);
    }

    // Check status
    if (invitation.status === "accepted") {
      return c.json(
        {
          error: "This invitation has already been accepted",
          code: "ACCEPTED",
        },
        410
      );
    }

    if (invitation.status === "revoked") {
      return c.json(
        { error: "This invitation has been revoked", code: "REVOKED" },
        410
      );
    }

    if (invitation.status === "expired" || invitation.expiresAt < new Date()) {
      return c.json(
        { error: "This invitation has expired", code: "EXPIRED" },
        410
      );
    }

    // Return public info (no sensitive data)
    return c.json({
      organization_name: invitation.organization.name,
      organization_slug: invitation.organization.slug,
      role: invitation.role,
      expires_at: invitation.expiresAt.toISOString(),
      email: invitation.email,
    });
  } finally {
    await client.end();
  }
});

/**
 * POST /invitations/accept - Accept invitation
 * Requires authentication and GitHub identity linked
 */
invitationRoutes.post("/accept", async (c) => {
  // Manual auth check since this route needs special handling
  const auth = c.get("auth");
  if (!auth?.userId) {
    return c.json({ error: "Authentication required" }, 401);
  }

  const body = await c.req.json<{ token: string }>();
  const { token } = body;

  if (!token) {
    return c.json({ error: "Token is required" }, 400);
  }

  const { db, client } = await createDb(c.env);
  try {
    // Find invitation
    const invitation = await db.query.invitations.findFirst({
      where: eq(invitations.token, token),
      with: { organization: true },
    });

    if (!invitation) {
      return c.json({ error: "Invitation not found" }, 404);
    }

    // Validate invitation status
    if (invitation.status === "accepted") {
      return c.json(
        { error: "This invitation has already been accepted" },
        400
      );
    }

    if (invitation.status === "revoked") {
      return c.json({ error: "This invitation has been revoked" }, 400);
    }

    if (invitation.status === "expired" || invitation.expiresAt < new Date()) {
      // Update status to expired if not already
      if (invitation.status !== "expired") {
        await db
          .update(invitations)
          .set({ status: "expired", updatedAt: new Date() })
          .where(eq(invitations.id, invitation.id));
      }
      return c.json({ error: "This invitation has expired" }, 400);
    }

    // Verify GitHub identity is linked
    const githubIdentity = await getVerifiedGitHubIdentity(
      auth.userId,
      c.env.WORKOS_API_KEY
    );

    if (!githubIdentity) {
      return c.json(
        {
          error: "GitHub account not linked",
          code: "GITHUB_NOT_LINKED",
          message:
            "Please link your GitHub account before accepting the invitation",
        },
        403
      );
    }

    // Check if user is already a member
    const existingMember = await db.query.organizationMembers.findFirst({
      where: and(
        eq(organizationMembers.organizationId, invitation.organizationId),
        eq(organizationMembers.userId, auth.userId)
      ),
    });

    if (existingMember) {
      // Mark invitation as accepted anyway to clean up
      await db
        .update(invitations)
        .set({
          status: "accepted",
          acceptedAt: new Date(),
          acceptedByUserId: auth.userId,
          updatedAt: new Date(),
        })
        .where(eq(invitations.id, invitation.id));

      return c.json(
        { error: "You are already a member of this organization" },
        409
      );
    }

    // Create organization membership
    await db.insert(organizationMembers).values({
      id: crypto.randomUUID(),
      organizationId: invitation.organizationId,
      userId: auth.userId,
      role: invitation.role,
      providerUserId: githubIdentity.userId,
      providerUsername: githubIdentity.username,
      providerLinkedAt: new Date(),
    });

    // Mark invitation as accepted
    await db
      .update(invitations)
      .set({
        status: "accepted",
        acceptedAt: new Date(),
        acceptedByUserId: auth.userId,
        updatedAt: new Date(),
      })
      .where(eq(invitations.id, invitation.id));

    console.log(
      `[invitations] ${githubIdentity.username} accepted invitation to ${invitation.organization.slug} as ${invitation.role}`
    );

    return c.json({
      success: true,
      organization_id: invitation.organizationId,
      organization_name: invitation.organization.name,
      organization_slug: invitation.organization.slug,
      role: invitation.role,
    });
  } finally {
    await client.end();
  }
});

export default invitationRoutes;
