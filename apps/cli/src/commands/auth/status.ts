import { defineCommand } from "citty";
import { getMe } from "../../lib/api.js";
import {
  decodeUserInfo,
  getAccessToken,
  getExpiresAt,
} from "../../lib/auth.js";
import { isTokenExpired, loadCredentials } from "../../lib/credentials.js";

export const statusCommand = defineCommand({
  meta: {
    name: "status",
    description: "Show current authentication status",
  },
  run: async () => {
    const credentials = loadCredentials();

    if (!credentials) {
      console.log("Not logged in.");
      console.log("Run `detent auth login` to authenticate.");
      return;
    }

    const userInfo = decodeUserInfo(credentials.access_token);
    const expiresAt = getExpiresAt(credentials.access_token);
    const expired = isTokenExpired(credentials);

    console.log("\nAuthentication Status\n");
    console.log("-".repeat(40));

    if (userInfo.email) {
      console.log(`Email:       ${userInfo.email}`);
    }

    if (userInfo.first_name || userInfo.last_name) {
      const name = [userInfo.first_name, userInfo.last_name]
        .filter(Boolean)
        .join(" ");
      console.log(`Name:        ${name}`);
    }

    console.log(`User ID:     ${userInfo.sub}`);

    if (userInfo.org_id) {
      console.log(`Org ID:      ${userInfo.org_id}`);
    }

    if (expiresAt) {
      const status = expired ? " (expired)" : "";
      console.log(`Expires:     ${expiresAt.toLocaleString()}${status}`);
    }

    console.log("-".repeat(40));

    // Fetch GitHub identity from API if token is not expired
    if (expired) {
      console.log(
        "\nNote: Token is expired. Run `detent auth login` to refresh."
      );
    } else {
      try {
        const accessToken = await getAccessToken();
        const me = await getMe(accessToken);

        if (me.github_linked && me.github_username) {
          console.log(`\nGitHub:      @${me.github_username} (linked)`);
        } else {
          console.log("\nGitHub:      Not linked");
        }
      } catch {
        // API call failed, skip GitHub status
      }
    }

    console.log("");
  },
});
