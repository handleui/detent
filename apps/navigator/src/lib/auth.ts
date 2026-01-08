import { jwtVerify, SignJWT } from "jose";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

const JWT_ISSUER = "detent-navigator";
const JWT_AUDIENCE = "detent-app";
const STATE_COOKIE_NAME = "oauth_state";
const SESSION_COOKIE_NAME = "session";

const getJwtSecretKey = () => {
  const secret = process.env.JWT_SECRET_KEY;
  if (!secret) {
    throw new Error("JWT_SECRET_KEY is not set");
  }
  return new Uint8Array(Buffer.from(secret, "base64"));
};

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
 * Store OAuth state in a short-lived httpOnly cookie
 */
export const setOAuthStateCookie = async (state: string) => {
  const cookieStore = await cookies();
  cookieStore.set({
    name: STATE_COOKIE_NAME,
    value: state,
    httpOnly: true,
    path: "/",
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    maxAge: 60 * 10, // 10 minutes - OAuth flow should complete quickly
  });
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

export const signOut = async () => {
  "use server";
  const cookieStore = await cookies();
  cookieStore.delete(SESSION_COOKIE_NAME);
  redirect("/login");
};
