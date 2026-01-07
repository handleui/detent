import { defineCommand } from "citty";
import { clearCredentials, isLoggedIn } from "../../lib/credentials.js";

export const logoutCommand = defineCommand({
  meta: {
    name: "logout",
    description: "Log out from your Detent account",
  },
  run: () => {
    if (!isLoggedIn()) {
      console.log("Not currently logged in.");
      return;
    }

    const cleared = clearCredentials();

    if (cleared) {
      console.log("Successfully logged out.");
    } else {
      console.error("Failed to clear credentials.");
      process.exit(1);
    }
  },
});
