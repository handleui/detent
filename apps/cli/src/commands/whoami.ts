/**
 * Whoami command - shows current user identity
 *
 * A quick way to check who you're logged in as.
 */

import { defineCommand } from "citty";
import { decodeJwt } from "jose";
import { getMe } from "../lib/api.js";
import { getAccessToken } from "../lib/auth.js";

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
    } catch {
      console.error("Not logged in. Run `detent auth login` first.");
      process.exit(1);
    }

    // Debug mode: show token claims
    if (args.debug) {
      try {
        const claims = decodeJwt(accessToken);
        console.log("Token claims:");
        console.log(`  iss: ${claims.iss}`);
        console.log(`  aud: ${claims.aud}`);
        console.log(`  sub: ${claims.sub}`);
        console.log(
          `  exp: ${claims.exp ? new Date(claims.exp * 1000).toISOString() : "N/A"}`
        );
        console.log(
          `  API URL: ${process.env.DETENT_API_URL ?? "https://api.detent.dev"}`
        );
        console.log();
      } catch (e) {
        console.error("Failed to decode token:", e);
      }
    }

    try {
      const me = await getMe(accessToken);

      const name = [me.first_name, me.last_name].filter(Boolean).join(" ");

      if (name) {
        console.log(`${name} <${me.email}>`);
      } else {
        console.log(me.email);
      }

      if (me.github_linked && me.github_username) {
        console.log(`GitHub: @${me.github_username}`);
      }
    } catch (error) {
      console.error(
        "Failed to fetch user info:",
        error instanceof Error ? error.message : String(error)
      );
      process.exit(1);
    }
  },
});
