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
import { ANSI_RESET, colors, hexToAnsi } from "../../tui/styles.js";

const brand = hexToAnsi(colors.brand);

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
  const auth = await requestDeviceAuthorization().catch((error: unknown) =>
    handleDeviceAuthError(error)
  );

  console.log("To authenticate, visit:");
  console.log(`  ${brand}${auth.verification_uri_complete}${ANSI_RESET}\n`);
  console.log(`Or enter code: ${brand}${auth.user_code}${ANSI_RESET}\n`);

  const tokens = await pollForTokens(auth.device_code, auth.interval).catch(
    (error: unknown) => {
      console.error(
        "\nAuthentication failed:",
        error instanceof Error ? error.message : String(error)
      );
      process.exit(1);
    }
  );

  return tokens;
};

const runNavigatorFlow = async (): Promise<TokenResponse> => {
  console.log("Opening browser...");

  try {
    return await authenticateViaNavigator();
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
    const email = `${brand}${identity.email}${ANSI_RESET}`;

    if (identity.github_username) {
      console.log(`Logged in as ${email} (@${identity.github_username})`);
    } else {
      console.log(`Logged in as ${email}`);
    }
  } catch {
    console.log("Logged in successfully");
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
