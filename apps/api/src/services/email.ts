import { Resend } from "resend";
import type { Env } from "../types/env.js";

interface SendInvitationEmailParams {
  to: string;
  organizationName: string;
  inviterName?: string;
  role: string;
  acceptUrl: string;
  expiresAt: Date;
}

export const createEmailService = (env: Env) => {
  const resend = new Resend(env.RESEND_API_KEY);

  return {
    sendInvitationEmail: async (params: SendInvitationEmailParams) => {
      const { to, organizationName, inviterName, role, acceptUrl, expiresAt } =
        params;

      const expiresInDays = Math.ceil(
        (expiresAt.getTime() - Date.now()) / (1000 * 60 * 60 * 24)
      );

      const { data, error } = await resend.emails.send({
        from: "Detent <noreply@detent.dev>",
        to,
        subject: `You've been invited to join ${organizationName} on Detent`,
        html: `
          <div style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto;">
            <h2 style="color: #1a1a1a;">You've been invited to join ${organizationName}</h2>
            <p style="color: #4a4a4a; line-height: 1.6;">
              ${inviterName ? `${inviterName} has` : "You have been"} invited to join
              <strong>${organizationName}</strong> as a <strong>${role}</strong>.
            </p>
            <p style="margin: 24px 0;">
              <a href="${acceptUrl}"
                 style="background: #000; color: #fff; padding: 12px 24px;
                        text-decoration: none; border-radius: 6px; display: inline-block;">
                Accept Invitation
              </a>
            </p>
            <p style="color: #888; font-size: 14px;">
              This invitation expires in ${expiresInDays} day${expiresInDays === 1 ? "" : "s"}.
            </p>
            <hr style="border: none; border-top: 1px solid #eee; margin: 24px 0;" />
            <p style="color: #888; font-size: 12px;">
              If you didn't expect this invitation, you can safely ignore this email.
            </p>
          </div>
        `,
        text: `
You've been invited to join ${organizationName}

${inviterName ? `${inviterName} has` : "You have been"} invited to join ${organizationName} as a ${role}.

Accept the invitation: ${acceptUrl}

This invitation expires in ${expiresInDays} day${expiresInDays === 1 ? "" : "s"}.

If you didn't expect this invitation, you can safely ignore this email.
        `.trim(),
      });

      if (error) {
        throw new Error(`Failed to send invitation email: ${error.message}`);
      }

      return data;
    },
  };
};
