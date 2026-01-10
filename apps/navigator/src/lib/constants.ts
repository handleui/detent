/**
 * Shared constants for auth that can be used in both server and client components
 */

export const VERIFICATION_CODE_LENGTH = 6;

export const COOKIE_NAMES = {
  session: "session",
  /** WorkOS sealed session cookie - contains encrypted refresh token */
  workosSession: "wos_session",
  oauthState: "oauth_state",
  pendingVerification: "pending_verification",
  /** CLI auth params cookie - stores port and state for CLI OAuth flow */
  cliAuthParams: "cli_auth_params",
  /** Return URL after authentication */
  returnTo: "return_to",
} as const;

export const AUTH_DURATIONS = {
  /** Session cookie max age in seconds (24 hours) */
  sessionMaxAgeSec: 60 * 60 * 24,
  /** OAuth state cookie max age in seconds (10 minutes) */
  oauthStateMaxAgeSec: 60 * 10,
  /** Pending verification cookie max age in seconds (10 minutes) */
  pendingVerificationMaxAgeSec: 60 * 10,
  /** Pending verification expiry in milliseconds (10 minutes) */
  pendingVerificationMs: 10 * 60 * 1000,
  /** CLI auth params cookie max age in seconds (10 minutes) */
  cliAuthParamsMaxAgeSec: 60 * 10,
} as const;
