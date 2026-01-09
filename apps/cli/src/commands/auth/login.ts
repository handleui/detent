import { defineCommand } from "citty";
import { syncIdentity } from "../../lib/api.js";
import {
  authenticateViaNavigator,
  getJwtExpiration,
  pollForTokens,
  requestDeviceAuthorization,
  type TokenResponse,
} from "../../lib/auth.js";
import type { Credentials } from "../../lib/credentials.js";
import { isLoggedIn, saveCredentials } from "../../lib/credentials.js";

const handleDeviceAuthError = (error: unknown): never => {
  if (error instanceof Error && error.message.includes("WORKOS_CLIENT_ID")) {
    console.error(`Error: ${error.message}`);
  } else if (error instanceof Error && error.message.includes("fetch")) {
    console.error("Network error: Unable to connect to authentication server.");
    console.error("Please check your internet connection and try again.");
  } else {
    console.error(
      "Failed to start authentication:",
      error instanceof Error ? error.message : String(error)
    );
  }
  process.exit(1);
};

const runHeadlessFlow = async (): Promise<TokenResponse> => {
  console.log("Requesting device authorization...\n");

  const auth = await requestDeviceAuthorization().catch((error: unknown) =>
    handleDeviceAuthError(error)
  );

  console.log("To authenticate, visit:");
  console.log(`  ${auth.verification_uri_complete}\n`);
  console.log(
    `Or go to ${auth.verification_uri} and enter code: ${auth.user_code}\n`
  );
  console.log("Waiting for authentication...");

  const tokens = await pollForTokens(auth.device_code, auth.interval, () => {
    process.stdout.write(".");
  }).catch((error: unknown) => {
    console.log("\n");
    console.error(
      "Authentication failed:",
      error instanceof Error ? error.message : String(error)
    );
    process.exit(1);
  });

  console.log("\n");
  return tokens;
};

const runNavigatorFlow = async (): Promise<TokenResponse> => {
  console.log("Opening browser to authenticate...\n");

  try {
    return await authenticateViaNavigator((status) => {
      console.log(status);
    });
  } catch (error) {
    console.error(
      "Authentication failed:",
      error instanceof Error ? error.message : String(error)
    );
    console.log(
      "\nTip: Use --headless flag if you're in an environment without browser access."
    );
    process.exit(1);
  }
};

const showLoginSuccess = async (accessToken: string): Promise<void> => {
  try {
    const identity = await syncIdentity(accessToken);

    if (identity.github_username) {
      console.log(
        `Successfully logged in as ${identity.email} (GitHub: @${identity.github_username})`
      );
    } else {
      console.log(`Successfully logged in as ${identity.email}`);
    }
  } catch {
    console.log("Successfully logged in!");
  }
};

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
    headless: {
      type: "boolean",
      description:
        "Use device code flow (for environments without browser access)",
      default: false,
    },
  },
  run: async ({ args }) => {
    if (!args.force && isLoggedIn()) {
      console.log("Already logged in. Use --force to re-authenticate.");
      return;
    }

    const tokens = args.headless
      ? await runHeadlessFlow()
      : await runNavigatorFlow();

    const credentials: Credentials = {
      access_token: tokens.access_token,
      refresh_token: tokens.refresh_token,
      expires_at: getJwtExpiration(tokens.access_token),
    };

    saveCredentials(credentials);
    await showLoginSuccess(tokens.access_token);
  },
});
