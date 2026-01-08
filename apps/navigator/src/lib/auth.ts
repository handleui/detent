import { jwtVerify, SignJWT } from "jose";
import { cookies } from "next/headers";
import { COOKIE_NAMES as CookieNames } from "./constants";

// Auth constants (internal)
const JWT_ISSUER = "detent-navigator";
const JWT_AUDIENCE = "detent-app";

// Local aliases
const STATE_COOKIE_NAME = CookieNames.oauthState;
const SESSION_COOKIE_NAME = CookieNames.session;
const PENDING_VERIFICATION_COOKIE = CookieNames.pendingVerification;

const getJwtSecretKey = () => {
  const secret = process.env.JWT_SECRET_KEY;
  if (!secret) {
    throw new Error("JWT_SECRET_KEY is not set");
  }
  return new Uint8Array(Buffer.from(secret, "base64"));
};

/**
 * Get WorkOS cookie password for sealed sessions
 * Used to encrypt refresh tokens in the session cookie
 * Must be at least 32 characters long for security
 */
export const getWorkOSCookiePassword = () => {
  const password = process.env.WORKOS_COOKIE_PASSWORD;
  if (!password) {
    throw new Error("WORKOS_COOKIE_PASSWORD is not set");
  }
  return password;
};

/**
 * Get WorkOS client ID from environment
 * Centralized to avoid duplication across auth routes
 */
export const getWorkOSClientId = () => {
  const clientId = process.env.WORKOS_CLIENT_ID;
  if (!clientId) {
    throw new Error("WORKOS_CLIENT_ID is not set");
  }
  return clientId;
};

/**
 * Common cookie options for auth-related cookies
 */
export interface CookieOptions {
  name: string;
  value: string;
  maxAge: number;
}

export const createSecureCookieOptions = ({
  name,
  value,
  maxAge,
}: CookieOptions) => ({
  name,
  value,
  httpOnly: true,
  path: "/",
  secure: process.env.NODE_ENV === "production",
  sameSite: "lax" as const,
  maxAge,
});

export interface WorkOSUser {
  id: string;
  email: string;
  firstName: string | null;
  lastName: string | null;
  profilePictureUrl: string | null;
}

/**
 * Generate a cryptographically secure random state for OAuth CSRF protection
 */
export const generateOAuthState = () => {
  const array = new Uint8Array(32);
  crypto.getRandomValues(array);
  return Array.from(array, (byte) => byte.toString(16).padStart(2, "0")).join(
    ""
  );
};

/**
 * Verify OAuth state matches the stored cookie and clear it
 */
export const verifyAndClearOAuthState = async (
  state: string | null
): Promise<boolean> => {
  const cookieStore = await cookies();
  const storedState = cookieStore.get(STATE_COOKIE_NAME)?.value;

  // Always clear the state cookie
  cookieStore.delete(STATE_COOKIE_NAME);

  if (!(state && storedState)) {
    return false;
  }

  // Constant-time comparison to prevent timing attacks
  if (state.length !== storedState.length) {
    return false;
  }

  let result = 0;
  for (let i = 0; i < state.length; i++) {
    // biome-ignore lint/suspicious/noBitwiseOperators: timing-safe comparison to prevent timing attacks
    result |= state.charCodeAt(i) ^ storedState.charCodeAt(i);
  }

  return result === 0;
};

/**
 * Create a signed JWT session token with proper security claims
 */
export const createSession = (user: WorkOSUser) => {
  return new SignJWT({ user })
    .setProtectedHeader({ alg: "HS256", typ: "JWT" })
    .setIssuedAt()
    .setIssuer(JWT_ISSUER)
    .setAudience(JWT_AUDIENCE)
    .setExpirationTime("24h") // 24 hours - balance of security and UX
    .sign(getJwtSecretKey());
};

/**
 * Verify a session token with issuer and audience validation
 */
export const verifySession = async (token: string) => {
  try {
    const { payload } = await jwtVerify(token, getJwtSecretKey(), {
      issuer: JWT_ISSUER,
      audience: JWT_AUDIENCE,
    });
    return payload;
  } catch {
    return null;
  }
};

export const getUser = async () => {
  const cookieStore = await cookies();
  const token = cookieStore.get(SESSION_COOKIE_NAME)?.value;

  if (token) {
    const payload = await verifySession(token);
    if (payload) {
      return { isAuthenticated: true, user: payload.user as WorkOSUser };
    }
  }

  return { isAuthenticated: false, user: null };
};

/**
 * Pending verification data stored in cookie during email verification flow
 * Note: WorkOS provides emailVerificationId (not userId) in the error response
 */
export interface PendingVerification {
  pendingAuthenticationToken: string;
  email: string;
  emailVerificationId: string;
  expiresAt: number;
}

/**
 * Create a signed JWT for pending verification data
 * This protects the pendingAuthenticationToken from tampering
 */
export const createPendingVerificationToken = (
  data: Omit<PendingVerification, "expiresAt">
) => {
  return new SignJWT({ ...data })
    .setProtectedHeader({ alg: "HS256", typ: "JWT" })
    .setIssuedAt()
    .setIssuer(JWT_ISSUER)
    .setAudience("pending-verification")
    .setExpirationTime("10m") // 10 minutes
    .sign(getJwtSecretKey());
};

/**
 * Get pending verification data from cookie
 * Returns null if cookie is missing, invalid, or expired
 * Cookie value is a signed JWT to prevent tampering with sensitive tokens
 */
export const getPendingVerification =
  async (): Promise<PendingVerification | null> => {
    const cookieStore = await cookies();
    const cookie = cookieStore.get(PENDING_VERIFICATION_COOKIE)?.value;

    if (!cookie) {
      return null;
    }

    try {
      const { payload } = await jwtVerify(cookie, getJwtSecretKey(), {
        issuer: JWT_ISSUER,
        audience: "pending-verification",
      });

      return {
        pendingAuthenticationToken:
          payload.pendingAuthenticationToken as string,
        email: payload.email as string,
        emailVerificationId: payload.emailVerificationId as string,
        expiresAt: (payload.exp ?? 0) * 1000, // Convert to milliseconds
      };
    } catch {
      cookieStore.delete(PENDING_VERIFICATION_COOKIE);
      return null;
    }
  };

/**
 * Clear pending verification cookie after successful verification or expiry
 */
export const clearPendingVerification = async () => {
  const cookieStore = await cookies();
  cookieStore.delete(PENDING_VERIFICATION_COOKIE);
};

/**
 * Mask email for display (e.g., "john@example.com" → "j***@example.com")
 */
export const maskEmail = (email: string): string => {
  const [local, domain] = email.split("@");
  if (!(local && domain)) {
    return "***";
  }
  if (local.length <= 1) {
    return `*@${domain}`;
  }
  return `${local[0]}***@${domain}`;
};
