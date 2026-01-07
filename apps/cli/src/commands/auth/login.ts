import { defineCommand } from "citty";
import { syncIdentity } from "../../lib/api.js";
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
    if (!args.force && isLoggedIn()) {
      console.log("Already logged in. Use --force to re-authenticate.");
      return;
    }

    console.log("Requesting device authorization...\n");

    let auth: Awaited<ReturnType<typeof requestDeviceAuthorization>>;
    try {
      auth = await requestDeviceAuthorization();
    } catch (error) {
      if (
        error instanceof Error &&
        error.message.includes("WORKOS_CLIENT_ID")
      ) {
        console.error(`Error: ${error.message}`);
      } else if (error instanceof Error && error.message.includes("fetch")) {
        console.error(
          "Network error: Unable to connect to authentication server."
        );
        console.error("Please check your internet connection and try again.");
      } else {
        console.error(
          "Failed to start authentication:",
          error instanceof Error ? error.message : String(error)
        );
      }
      process.exit(1);
    }

    console.log("To authenticate, visit:");
    console.log(`  ${auth.verification_uri_complete}\n`);
    console.log(
      `Or go to ${auth.verification_uri} and enter code: ${auth.user_code}\n`
    );
    console.log("Waiting for authentication...");

    let tokens: Awaited<ReturnType<typeof pollForTokens>>;
    try {
      tokens = await pollForTokens(auth.device_code, auth.interval, () => {
        process.stdout.write(".");
      });
    } catch (error) {
      console.log("\n");
      console.error(
        "Authentication failed:",
        error instanceof Error ? error.message : String(error)
      );
      process.exit(1);
    }

    console.log("\n");

    const expiresInMs = (tokens.expires_in ?? 3600) * 1000;
    const credentials: Credentials = {
      access_token: tokens.access_token,
      refresh_token: tokens.refresh_token,
      expires_at: Date.now() + expiresInMs,
    };

    saveCredentials(credentials);

    // Sync identity from WorkOS to capture GitHub info if available
    try {
      const identity = await syncIdentity(tokens.access_token);

      if (identity.github_username) {
        console.log(
          `Successfully logged in as ${identity.email} (GitHub: @${identity.github_username})`
        );
      } else {
        console.log(`Successfully logged in as ${identity.email}`);
      }
    } catch {
      // Identity sync failed, but login succeeded - show basic message
      console.log("Successfully logged in!");
    }
  },
});
