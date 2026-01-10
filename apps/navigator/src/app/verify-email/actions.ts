"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import {
  clearPendingVerification,
  createSecureCookieOptions,
  createSession,
  getPendingVerification,
  getWorkOSClientId,
} from "@/lib/auth";
import {
  AUTH_DURATIONS,
  COOKIE_NAMES,
  VERIFICATION_CODE_LENGTH,
} from "@/lib/constants";
import { workos } from "@/lib/workos";

export interface VerifyState {
  error: string | null;
}

export interface ResendState {
  success: boolean;
  error: string | null;
}

/**
 * Type guard for WorkOS exceptions with code property
 * WorkOS SDK throws exceptions with status and optional code for specific errors
 */
interface WorkOSException extends Error {
  status?: number;
  code?: string;
  rawData?: {
    code?: string;
  };
}

const getWorkOSErrorCode = (error: unknown): string | undefined => {
  if (typeof error !== "object" || error === null) {
    return undefined;
  }
  const err = error as WorkOSException;
  return err.code ?? err.rawData?.code;
};

const isRateLimitError = (error: unknown): boolean => {
  if (typeof error !== "object" || error === null) {
    return false;
  }
  const err = error as WorkOSException;
  return err.status === 429;
};

/**
 * Checks if the error indicates an invalid verification code
 * WorkOS returns "invalid_one_time_code" for invalid verification codes
 * @see https://workos.com/docs/events/directory-sync - authentication.email_verification_failed event
 */
const isInvalidCodeError = (error: unknown): boolean => {
  const code = getWorkOSErrorCode(error);
  if (
    code === "invalid_one_time_code" ||
    code === "invalid_code" ||
    code === "code_invalid"
  ) {
    return true;
  }
  if (error instanceof Error) {
    const msg = error.message.toLowerCase();
    return msg.includes("invalid") && msg.includes("code");
  }
  return false;
};

/**
 * Checks if the error indicates an expired token or code
 */
const isExpiredError = (error: unknown): boolean => {
  const code = getWorkOSErrorCode(error);
  if (
    code === "code_expired" ||
    code === "pending_authentication_token_expired"
  ) {
    return true;
  }
  if (error instanceof Error) {
    return error.message.toLowerCase().includes("expired");
  }
  return false;
};

/** Regex to validate verification codes based on configured length */
const VERIFICATION_CODE_REGEX = new RegExp(
  `^\\d{${VERIFICATION_CODE_LENGTH}}$`
);

export const verifyEmailCode = async (
  _prevState: VerifyState,
  formData: FormData
): Promise<VerifyState> => {
  const rawCode = formData.get("code");

  // Validate code is a string and contains exactly 6 digits
  if (typeof rawCode !== "string") {
    return { error: "Please enter a valid 6-digit code" };
  }

  const code = rawCode.trim();
  if (!VERIFICATION_CODE_REGEX.test(code)) {
    return { error: "Please enter a valid 6-digit code" };
  }

  const pending = await getPendingVerification();
  if (!pending) {
    redirect("/login?error=session_expired");
  }

  try {
    const { user } =
      await workos.userManagement.authenticateWithEmailVerification({
        clientId: getWorkOSClientId(),
        code,
        pendingAuthenticationToken: pending.pendingAuthenticationToken,
      });

    // Clear pending verification cookie
    await clearPendingVerification();

    // Create session
    const token = await createSession(user);
    const cookieStore = await cookies();
    cookieStore.set(
      createSecureCookieOptions({
        name: COOKIE_NAMES.session,
        value: token,
        maxAge: AUTH_DURATIONS.sessionMaxAgeSec,
      })
    );
  } catch (error) {
    // Check for rate limiting first (HTTP 429)
    if (isRateLimitError(error)) {
      return { error: "Too many attempts. Please wait a few minutes." };
    }

    // Handle invalid code errors
    if (isInvalidCodeError(error)) {
      return {
        error: "Invalid verification code. Please check and try again.",
      };
    }

    // Handle expired token/code errors
    if (isExpiredError(error)) {
      await clearPendingVerification();
      redirect("/login?error=verification_expired");
    }

    return { error: "Verification failed. Please try again." };
  }

  // Redirect after successful verification
  redirect("/");
};

export const resendVerificationEmail = async (
  _prevState: ResendState
): Promise<ResendState> => {
  const pending = await getPendingVerification();

  if (!pending) {
    redirect("/login?error=session_expired");
  }

  try {
    // WorkOS sendVerificationEmail requires userId, not email
    // We need to look up the user by email first
    const users = await workos.userManagement.listUsers({
      email: pending.email,
    });

    const user = users.data[0];
    if (!user) {
      return {
        success: false,
        error: "Could not find your account. Please try signing in again.",
      };
    }

    await workos.userManagement.sendVerificationEmail({
      userId: user.id,
    });

    return { success: true, error: null };
  } catch (error) {
    // Check for rate limiting (HTTP 429)
    if (isRateLimitError(error)) {
      return {
        success: false,
        error: "Please wait before requesting another code.",
      };
    }

    return {
      success: false,
      error: "Failed to resend code. Please try again.",
    };
  }
};
