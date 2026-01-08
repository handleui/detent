import { createSession, verifyAndClearOAuthState } from "@detent/lib/auth";
import { workos } from "@detent/lib/workos";
import { NextResponse } from "next/server";

const getClientId = () => {
  const clientId = process.env.WORKOS_CLIENT_ID;
  if (!clientId) {
    throw new Error("WORKOS_CLIENT_ID is not set");
  }
  return clientId;
};

export const GET = async (request: Request) => {
  const url = new URL(request.url);
  const code = url.searchParams.get("code");
  const state = url.searchParams.get("state");

  // Verify OAuth state to prevent CSRF attacks
  const isValidState = await verifyAndClearOAuthState(state);
  if (!isValidState) {
    console.error("OAuth state validation failed - possible CSRF attack");
    return NextResponse.redirect(
      new URL("/login?error=invalid_state", request.url)
    );
  }

  if (!code) {
    return NextResponse.redirect(new URL("/login?error=no_code", request.url));
  }

  try {
    const { user } = await workos.userManagement.authenticateWithCode({
      clientId: getClientId(),
      code,
    });

    const token = await createSession(user);

    const response = NextResponse.redirect(new URL("/", request.url));
    response.cookies.set({
      name: "session",
      value: token,
      httpOnly: true,
      path: "/",
      secure: process.env.NODE_ENV === "production",
      sameSite: "lax",
      maxAge: 60 * 60 * 24, // 24 hours - matches JWT expiration
    });

    return response;
  } catch (error) {
    console.error("Auth error:", error);
    return NextResponse.redirect(
      new URL("/login?error=auth_failed", request.url)
    );
  }
};
