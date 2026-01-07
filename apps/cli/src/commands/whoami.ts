/**
 * Whoami command - shows current user identity
 *
 * A quick way to check who you're logged in as.
 */

import { defineCommand } from "citty";
import { decodeJwt } from "jose";
import type { MeResponse } from "../lib/api.js";
import { getMe } from "../lib/api.js";
import { getAccessToken } from "../lib/auth.js";

const handleAuthError = (error: unknown, debug: boolean): never => {
  const message = error instanceof Error ? error.message : String(error);

  if (message.includes("Not logged in")) {
    console.error("Not logged in. Run `detent auth login` first.");
  } else if (message.includes("refresh")) {
    console.error(
      "Session expired and refresh failed. Run `detent auth login` to re-authenticate."
    );
    if (debug) {
      console.error("  Error:", message);
    }
  } else {
    console.error("Authentication error:", message);
  }

  process.exit(1);
};

const displayTokenClaims = (accessToken: string): void => {
  try {
    const claims = decodeJwt(accessToken);
    const expiry = claims.exp
      ? new Date(claims.exp * 1000).toISOString()
      : "N/A";
    const apiUrl = process.env.DETENT_API_URL ?? "https://api.detent.dev";

    console.log("Token claims:");
    console.log(`  iss: ${claims.iss}`);
    console.log(`  aud: ${claims.aud}`);
    console.log(`  sub: ${claims.sub}`);
    console.log(`  exp: ${expiry}`);
    console.log(`  API URL: ${apiUrl}`);
    console.log();
  } catch (e) {
    console.error("Failed to decode token:", e);
  }
};

const displayUserInfo = (me: MeResponse): void => {
  const name = [me.first_name, me.last_name].filter(Boolean).join(" ");

  if (name) {
    console.log(`${name} <${me.email}>`);
  } else {
    console.log(me.email);
  }

  if (me.github_linked && me.github_username) {
    console.log(`GitHub: @${me.github_username}`);
  }
};

export const whoamiCommand = defineCommand({
  meta: {
    name: "whoami",
    description: "Show current user identity",
  },
  args: {
    debug: {
      type: "boolean",
      description: "Show debug information about the access token",
      default: false,
    },
  },
  run: async ({ args }) => {
    let accessToken: string;
    try {
      accessToken = await getAccessToken();
    } catch (error) {
      return handleAuthError(error, args.debug);
    }

    if (args.debug) {
      displayTokenClaims(accessToken);
    }

    try {
      const me = await getMe(accessToken);
      displayUserInfo(me);
    } catch (error) {
      console.error(
        "Failed to fetch user info:",
        error instanceof Error ? error.message : String(error)
      );
      process.exit(1);
    }
  },
});
