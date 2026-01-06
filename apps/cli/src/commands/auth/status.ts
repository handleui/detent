import { findGitRoot } from "@detent/git";
import { defineCommand } from "citty";
import { decodeUserInfo, getExpiresAt } from "../../lib/auth.js";
import { isTokenExpired, loadCredentials } from "../../lib/credentials.js";

export const statusCommand = defineCommand({
  meta: {
    name: "status",
    description: "Show current authentication status",
  },
  run: async () => {
    const repoRoot = await findGitRoot(process.cwd());
    if (!repoRoot) {
      console.error("Not in a git repository.");
      process.exit(1);
    }
    const credentials = loadCredentials(repoRoot);

    if (!credentials) {
      console.log("Not logged in.");
      console.log("Run `detent auth login` to authenticate.");
      return;
    }

    const userInfo = decodeUserInfo(credentials.access_token);
    const expiresAt = getExpiresAt(credentials.access_token);
    const expired = isTokenExpired(credentials);

    console.log("Logged in as:");
    console.log(`  User ID: ${userInfo.sub}`);

    if (userInfo.email) {
      console.log(`  Email: ${userInfo.email}`);
    }

    if (userInfo.first_name || userInfo.last_name) {
      const name = [userInfo.first_name, userInfo.last_name]
        .filter(Boolean)
        .join(" ");
      console.log(`  Name: ${name}`);
    }

    if (userInfo.org_id) {
      console.log(`  Organization: ${userInfo.org_id}`);
    }

    if (expiresAt) {
      const status = expired ? " (expired)" : "";
      console.log(`  Token expires: ${expiresAt.toLocaleString()}${status}`);
    }
  },
});
