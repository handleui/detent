import { jwtDecrypt } from "jose";
import { NextResponse } from "next/server";
import { getWorkOSCookiePassword } from "@/lib/auth";

interface TokenPayload {
  accessToken: string;
  refreshToken: string;
  exp: number;
}

interface TokenRequestBody {
  code: string;
}

/**
 * API route to exchange encrypted one-time code for tokens
 * This is called by the CLI after receiving the encrypted code from the callback
 */
export const POST = async (request: Request) => {
  try {
    const body = (await request.json()) as TokenRequestBody;
    const { code } = body;

    if (!code) {
      return NextResponse.json(
        { error: "missing_code", message: "Code is required" },
        { status: 400 }
      );
    }

    // Decrypt the one-time code
    // Use SHA-256 hash of password for consistent 256-bit key (matches authorize route)
    const password = getWorkOSCookiePassword();
    const passwordBytes = new TextEncoder().encode(password);
    const hashBuffer = await crypto.subtle.digest("SHA-256", passwordBytes);
    const encryptionKey = new Uint8Array(hashBuffer);

    let payload: TokenPayload;
    try {
      const result = await jwtDecrypt(code, encryptionKey);
      payload = result.payload as unknown as TokenPayload;
    } catch {
      return NextResponse.json(
        { error: "invalid_code", message: "Invalid or expired code" },
        { status: 400 }
      );
    }

    // Verify expiration (jose already handles this, but double-check)
    const now = Math.floor(Date.now() / 1000);
    if (payload.exp && payload.exp < now) {
      return NextResponse.json(
        { error: "expired_code", message: "Code has expired" },
        { status: 400 }
      );
    }

    // Extract tokens
    const { accessToken, refreshToken } = payload;

    if (!(accessToken && refreshToken)) {
      return NextResponse.json(
        { error: "invalid_payload", message: "Invalid token payload" },
        { status: 400 }
      );
    }

    // Return tokens with expiration info
    // WorkOS access tokens typically expire in 24 hours
    const expiresAt = new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString();

    return NextResponse.json({
      access_token: accessToken,
      refresh_token: refreshToken,
      expires_at: expiresAt,
    });
  } catch (err) {
    console.error("[api/cli/token] Error:", err);
    return NextResponse.json(
      { error: "server_error", message: "An unexpected error occurred" },
      { status: 500 }
    );
  }
};
