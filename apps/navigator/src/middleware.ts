import { jwtVerify } from "jose";
import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";

// Must match values in auth.ts
const JWT_ISSUER = "detent-navigator";
const JWT_AUDIENCE = "detent-app";

const PUBLIC_PATHS = ["/login", "/auth/callback"];

const getJwtSecretKey = () => {
  const secret = process.env.JWT_SECRET_KEY;
  if (!secret) {
    throw new Error("JWT_SECRET_KEY is not set");
  }
  return new Uint8Array(Buffer.from(secret, "base64"));
};

export const middleware = async (request: NextRequest) => {
  const { pathname } = request.nextUrl;

  // Allow public paths
  if (PUBLIC_PATHS.some((path) => pathname.startsWith(path))) {
    return NextResponse.next();
  }

  // Check session
  const token = request.cookies.get("session")?.value;

  if (!token) {
    return NextResponse.redirect(new URL("/login", request.url));
  }

  try {
    // Verify token with issuer and audience validation
    await jwtVerify(token, getJwtSecretKey(), {
      issuer: JWT_ISSUER,
      audience: JWT_AUDIENCE,
    });

    // SSRF protection: forward request headers
    // This mitigates SSRF issues in Next.js < 14.2.32 or < 15.4.7
    // Safe on all versions and recommended best practice
    const response = NextResponse.next({
      request: {
        headers: new Headers(request.headers),
      },
    });

    return response;
  } catch {
    // Invalid token, clear and redirect
    const response = NextResponse.redirect(new URL("/login", request.url));
    response.cookies.delete("session");
    return response;
  }
};

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
