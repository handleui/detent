import { findGitRoot } from "@detent/git";
import { defineCommand } from "citty";
import { pollForTokens, requestDeviceAuthorization } from "../../lib/auth.js";
import type { Credentials } from "../../lib/credentials.js";
import { isLoggedIn, saveCredentials } from "../../lib/credentials.js";

export const loginCommand = defineCommand({
  meta: {
    name: "login",
    description: "Authenticate with your Detent account",
  },
  args: {
    force: {
      type: "boolean",
      description: "Force re-authentication even if already logged in",
      default: false,
    },
  },
  run: async ({ args }) => {
    const repoRoot = await findGitRoot(process.cwd());
    if (!repoRoot) {
      console.error("Not in a git repository.");
      process.exit(1);
    }

    if (!args.force && isLoggedIn(repoRoot)) {
      console.log("Already logged in. Use --force to re-authenticate.");
      return;
    }

    console.log("Requesting device authorization...\n");

    const auth = await requestDeviceAuthorization();

    console.log("To authenticate, visit:");
    console.log(`  ${auth.verification_uri_complete}\n`);
    console.log(
      `Or go to ${auth.verification_uri} and enter code: ${auth.user_code}\n`
    );
    console.log("Waiting for authentication...");

    const tokens = await pollForTokens(auth.device_code, auth.interval, () => {
      process.stdout.write(".");
    });

    console.log("\n");

    const credentials: Credentials = {
      access_token: tokens.access_token,
      refresh_token: tokens.refresh_token,
      expires_at: Date.now() + tokens.expires_in * 1000,
    };

    saveCredentials(credentials, repoRoot);

    console.log("Successfully logged in!");
  },
});
