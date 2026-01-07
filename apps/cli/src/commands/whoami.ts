/**
 * Whoami command - shows current user identity
 *
 * A quick way to check who you're logged in as.
 */

import { defineCommand } from "citty";
import { getMe } from "../lib/api.js";
import { getAccessToken } from "../lib/auth.js";

export const whoamiCommand = defineCommand({
  meta: {
    name: "whoami",
    description: "Show current user identity",
  },
  run: async () => {
    let accessToken: string;
    try {
      accessToken = await getAccessToken();
    } catch {
      console.error("Not logged in. Run `detent auth login` first.");
      process.exit(1);
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
