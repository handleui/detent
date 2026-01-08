import { NextResponse } from "next/server";
import {
  createPendingVerificationToken,
  createSecureCookieOptions,
  createSession,
  getWorkOSClientId,
  getWorkOSCookiePassword,
  verifyAndClearOAuthState,
} from "@/lib/auth";
import { AUTH_DURATIONS, COOKIE_NAMES } from "@/lib/constants";
import { workos } from "@/lib/workos";

/**
 * Check if WorkOS sealed sessions are enabled
 * When enabled, WorkOS encrypts the refresh token into a sealed session cookie
 * This is more secure as it prevents token theft and enables server-side token refresh
 */
const isSealedSessionsEnabled = () => {
  try {
    getWorkOSCookiePassword();
    return true;
  } catch {
    return false;
  }
};

/**
 * Type guard for WorkOS email verification required error
 * This error is thrown when GitHub OAuth user hasn't verified their email
 *
 * Actual WorkOS error structure:
 * {
 *   code: "email_verification_required",
 *   message: "Email ownership must be verified before authentication.",
 *   email: "user@example.com",
 *   pending_authentication_token: "...",
 *   email_verification_id: "email_verification_..."
 * }
 *
 * Note: WorkOS auto-sends the verification email when this error occurs
 */
interface EmailVerificationRequiredError {
  code: string;
  rawData: {
    code: string;
    pending_authentication_token: string;
    email: string;
    email_verification_id: string;
  };
}

const isEmailVerificationError = (
  error: unknown
): error is EmailVerificationRequiredError => {
  if (typeof error !== "object" || error === null) {
    return false;
  }
  const err = error as Record<string, unknown>;
  if (err.code !== "email_verification_required" && err.rawData) {
    const rawData = err.rawData as Record<string, unknown>;
    return rawData.code === "email_verification_required";
  }
  return err.code === "email_verification_required";
};

/**
 * Set pending verification cookie for email verification flow
 * Uses signed JWT to protect the pendingAuthenticationToken from tampering
 */
const setPendingVerificationCookie = async (
  response: NextResponse,
  data: {
    pendingAuthenticationToken: string;
    email: string;
    emailVerificationId: string;
  }
) => {
  const signedToken = await createPendingVerificationToken(data);
  response.cookies.set(
    createSecureCookieOptions({
      name: COOKIE_NAMES.pendingVerification,
      value: signedToken,
      maxAge: AUTH_DURATIONS.pendingVerificationMaxAgeSec,
    })
  );
};

export const GET = async (request: Request) => {
  const url = new URL(request.url);
  const code = url.searchParams.get("code");
  const state = url.searchParams.get("state");
  const error = url.searchParams.get("error");
  const errorDescription = url.searchParams.get("error_description");

  // Check for OAuth errors from WorkOS
  if (error) {
    console.error("[auth/callback] OAuth error:", error, errorDescription);
    return NextResponse.redirect(
      new URL(
        `/login?error=${error}&message=${encodeURIComponent(errorDescription || "")}`,
        request.url
      )
    );
  }

  // Verify OAuth state to prevent CSRF attacks
  const isValidState = await verifyAndClearOAuthState(state);

  if (!isValidState) {
    return NextResponse.redirect(
      new URL("/login?error=invalid_state", request.url)
    );
  }

  if (!code) {
    return NextResponse.redirect(new URL("/login?error=no_code", request.url));
  }

  try {
    const shouldSealSession = isSealedSessionsEnabled();
    const authOptions = shouldSealSession
      ? {
          clientId: getWorkOSClientId(),
          code,
          session: {
            sealSession: true,
            cookiePassword: getWorkOSCookiePassword(),
          },
        }
      : {
          clientId: getWorkOSClientId(),
          code,
        };

    const authResponse = await workos.userManagement.authenticateWithCode(
      authOptions as Parameters<
        typeof workos.userManagement.authenticateWithCode
      >[0]
    );
    const { user } = authResponse;

    const response = NextResponse.redirect(new URL("/", request.url));

    // Store the custom session token for user data access
    const token = await createSession(user);
    response.cookies.set(
      createSecureCookieOptions({
        name: COOKIE_NAMES.session,
        value: token,
        maxAge: AUTH_DURATIONS.sessionMaxAgeSec,
      })
    );

    // If using sealed sessions, also store the WorkOS sealed session
    // This contains the encrypted refresh token for token refresh capability
    if (shouldSealSession && "sealedSession" in authResponse) {
      response.cookies.set(
        createSecureCookieOptions({
          name: COOKIE_NAMES.workosSession,
          value: authResponse.sealedSession as string,
          maxAge: AUTH_DURATIONS.sessionMaxAgeSec,
        })
      );
    }

    return response;
  } catch (error) {
    // Handle email verification required error (GitHub OAuth specific)
    if (isEmailVerificationError(error)) {
      const pendingData = {
        pendingAuthenticationToken: error.rawData.pending_authentication_token,
        email: error.rawData.email,
        emailVerificationId: error.rawData.email_verification_id,
      };

      const response = NextResponse.redirect(
        new URL("/verify-email", request.url)
      );
      await setPendingVerificationCookie(response, pendingData);
      return response;
    }

    console.error("[auth/callback] Auth error:", error);
    return NextResponse.redirect(
      new URL("/login?error=auth_failed", request.url)
    );
  }
};
