import { NextResponse } from "next/server";
import {
  createSecureCookieOptions,
  generateOAuthState,
  getWorkOSClientId,
  isValidReturnUrl,
} from "@/lib/auth";
import { AUTH_DURATIONS, COOKIE_NAMES } from "@/lib/constants";
import { workos } from "@/lib/workos";

/**
 * OAuth initiation endpoint - generates state, sets cookie, redirects to GitHub
 * Cookies can only be set in Route Handlers or Server Actions, not Server Components
 */
export const GET = async (request: Request) => {
  const url = new URL(request.url);
  const returnTo = url.searchParams.get("returnTo");

  // Generate cryptographically secure state for CSRF protection
  const state = generateOAuthState();

  const redirectUri = `${process.env.NEXT_PUBLIC_APP_URL || "http://localhost:3000"}/auth/callback`;

  // Build the GitHub OAuth URL via WorkOS (async - must await!)
  const authorizationUrl = await workos.userManagement.getAuthorizationUrl({
    provider: "GitHubOAuth",
    clientId: getWorkOSClientId(),
    redirectUri,
    state,
  });

  // Redirect to GitHub with state cookie set
  const response = NextResponse.redirect(authorizationUrl);

  // Set state cookie for CSRF verification on callback
  response.cookies.set(
    createSecureCookieOptions({
      name: COOKIE_NAMES.oauthState,
      value: state,
      maxAge: AUTH_DURATIONS.oauthStateMaxAgeSec,
    })
  );

  // Store returnTo URL only if it's a valid relative path (prevents open redirect)
  if (isValidReturnUrl(returnTo)) {
    response.cookies.set(
      createSecureCookieOptions({
        name: COOKIE_NAMES.returnTo,
        value: returnTo,
        maxAge: AUTH_DURATIONS.oauthStateMaxAgeSec,
      })
    );
  }

  return response;
};
