import { EncryptJWT } from "jose";
import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { getWorkOSCookiePassword } from "@/lib/auth";
import { COOKIE_NAMES } from "@/lib/constants";
import { workos } from "@/lib/workos";

/**
 * Generate HTML response for success page that displays briefly before redirect
 */
const generateSuccessHtml = (redirectUrl: string) => `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta http-equiv="refresh" content="0; url=${redirectUrl}">
  <title>Authorization Successful - Detent CLI</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: system-ui, -apple-system, sans-serif;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      background: #fff;
    }
    .container {
      text-align: center;
      padding: 2rem;
      max-width: 400px;
    }
    .icon {
      width: 48px;
      height: 48px;
      margin: 0 auto 1.5rem;
      color: #22c55e;
    }
    h1 {
      font-size: 1.25rem;
      font-weight: 600;
      color: #171717;
      margin-bottom: 0.5rem;
    }
    p {
      font-size: 0.875rem;
      color: #737373;
    }
  </style>
</head>
<body>
  <div class="container">
    <svg class="icon" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24" aria-hidden="true">
      <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
    </svg>
    <h1>Authorization Successful</h1>
    <p>You can close this window and return to the CLI.</p>
  </div>
</body>
</html>
`;

/**
 * Create encrypted one-time code containing tokens
 * Uses JWE with direct encryption (A256GCM) for secure token transport
 */
const createEncryptedCode = async (
  accessToken: string,
  refreshToken: string
) => {
  // A256GCM requires exactly 256 bits (32 bytes) key
  // Hash the password to get consistent 32-byte key
  const password = getWorkOSCookiePassword();
  const passwordBytes = new TextEncoder().encode(password);
  const hashBuffer = await crypto.subtle.digest("SHA-256", passwordBytes);
  const encryptionKey = new Uint8Array(hashBuffer);

  const encryptedCode = await new EncryptJWT({
    accessToken,
    refreshToken,
  })
    .setProtectedHeader({ alg: "dir", enc: "A256GCM" })
    .setExpirationTime("60s")
    .encrypt(encryptionKey);

  return encryptedCode;
};

/**
 * CLI Authorization endpoint
 * User must already be authenticated via Navigator
 * Generates encrypted tokens and redirects to CLI's localhost server
 */
export const GET = async (request: Request) => {
  const url = new URL(request.url);
  const port = url.searchParams.get("port");
  const state = url.searchParams.get("state");

  // Validate required parameters
  if (!(port && state)) {
    return NextResponse.redirect(
      new URL("/cli/auth?error=missing_params", request.url)
    );
  }

  // Get existing session
  const cookieStore = await cookies();
  const sealedSession = cookieStore.get(COOKIE_NAMES.workosSession)?.value;

  if (!sealedSession) {
    // No session, redirect back to CLI auth page which will handle login redirect
    const returnUrl = `/cli/auth?port=${encodeURIComponent(port)}&state=${encodeURIComponent(state)}`;
    return NextResponse.redirect(
      new URL(`/login?returnTo=${encodeURIComponent(returnUrl)}`, request.url)
    );
  }

  try {
    const cookiePassword = getWorkOSCookiePassword();
    const session = workos.userManagement.loadSealedSession({
      sessionData: sealedSession,
      cookiePassword,
    });

    // Refresh the session to get fresh tokens
    const refreshResult = await session.refresh({ cookiePassword });

    if (!(refreshResult.authenticated && refreshResult.session)) {
      // Session expired, redirect to login
      const returnUrl = `/cli/auth?port=${encodeURIComponent(port)}&state=${encodeURIComponent(state)}`;
      return NextResponse.redirect(
        new URL(`/login?returnTo=${encodeURIComponent(returnUrl)}`, request.url)
      );
    }

    const { accessToken, refreshToken } = refreshResult.session;
    if (!(accessToken && refreshToken)) {
      throw new Error("Missing tokens in session");
    }

    // Create encrypted one-time code
    const encryptedCode = await createEncryptedCode(accessToken, refreshToken);

    // Build redirect URL to CLI's localhost server
    const cliCallbackUrl = new URL(`http://localhost:${port}/callback`);
    cliCallbackUrl.searchParams.set("code", encryptedCode);
    cliCallbackUrl.searchParams.set("state", state);

    // Return HTML success page with redirect
    return new NextResponse(generateSuccessHtml(cliCallbackUrl.toString()), {
      status: 200,
      headers: {
        "Content-Type": "text/html",
      },
    });
  } catch (err) {
    console.error("[cli/auth/authorize] Error:", err);
    return NextResponse.redirect(
      new URL("/cli/auth?error=auth_failed", request.url)
    );
  }
};
