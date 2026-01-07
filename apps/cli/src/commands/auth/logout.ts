import { findGitRoot } from "@detent/git";
import { defineCommand } from "citty";
import { clearCredentials, isLoggedIn } from "../../lib/credentials.js";

export const logoutCommand = defineCommand({
  meta: {
    name: "logout",
    description: "Log out from your Detent account",
  },
  run: async () => {
    const repoRoot = await findGitRoot(process.cwd());
    if (!repoRoot) {
      console.error("Not in a git repository.");
      process.exit(1);
    }

    if (!isLoggedIn(repoRoot)) {
      console.log("Not currently logged in.");
      return;
    }

    const cleared = clearCredentials(repoRoot);

    if (cleared) {
      console.log("Successfully logged out.");
    } else {
      console.error("Failed to clear credentials.");
      process.exit(1);
    }
  },
});
