/**
 * GitHub OAuth linking command
 *
 * Links a user's GitHub account to their Detent team membership
 * using WorkOS GitHub OAuth flow with a local callback server.
 * Implements PKCE (Proof Key for Code Exchange) per OAuth 2.1.
 */

import { createHash, randomBytes } from "node:crypto";
import { createServer } from "node:http";
import { findGitRoot } from "@detent/git";
import { defineCommand } from "citty";
import {
  type AuthorizeResponse,
  getAuthorizeUrl,
  getTeams,
  submitCallback,
  type Team,
} from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";
import { findTeamByIdOrSlug, selectTeam } from "../../lib/ui.js";

// PKCE utilities per RFC 7636
const base64UrlEncode = (buffer: Buffer): string =>
  buffer.toString("base64url");

const generateCodeVerifier = (): string => {
  // Generate 32 random bytes, base64url encoded (43 chars)
  return base64UrlEncode(randomBytes(32));
};

const generateCodeChallenge = (verifier: string): string => {
  // SHA256 hash of verifier, base64url encoded
  const hash = createHash("sha256").update(verifier).digest();
  return base64UrlEncode(hash);
};

const findAvailablePort = async (startPort = 9876): Promise<number> => {
  const net = await import("node:net");

  const isPortAvailable = (port: number): Promise<boolean> =>
    new Promise((resolve) => {
      const server = net.createServer();
      server.once("error", () => resolve(false));
      server.once("listening", () => {
        server.close();
        resolve(true);
      });
      // Check on loopback only for security
      server.listen(port, "127.0.0.1");
    });

  let port = startPort;
  while (port < startPort + 100) {
    if (await isPortAvailable(port)) {
      return port;
    }
    port++;
  }
  throw new Error("Could not find an available port");
};

const openBrowser = async (url: string): Promise<void> => {
  const { exec } = await import("node:child_process");
  const { promisify } = await import("node:util");
  const execAsync = promisify(exec);

  const platform = process.platform;
  let command: string;
  if (platform === "darwin") {
    command = `open "${url}"`;
  } else if (platform === "win32") {
    command = `start "" "${url}"`;
  } else {
    command = `xdg-open "${url}"`;
  }

  await execAsync(command).catch(() => {
    // Ignore errors - user can manually open URL
  });
};

interface OAuthCallbackResult {
  code: string;
  state: string;
}

const waitForOAuthCallback = (
  port: number,
  expectedState: string
): Promise<OAuthCallbackResult> =>
  new Promise((resolve, reject) => {
    const server = createServer((req, res) => {
      const url = new URL(req.url ?? "", `http://127.0.0.1:${port}`);

      if (url.pathname === "/callback") {
        const code = url.searchParams.get("code");
        const state = url.searchParams.get("state");
        const error = url.searchParams.get("error");

        if (error) {
          res.writeHead(400, { "Content-Type": "text/html" });
          res.end(`
            <html>
              <body style="font-family: system-ui; padding: 40px; text-align: center;">
                <h1>Authorization Failed</h1>
                <p>${error}</p>
                <p>You can close this window.</p>
              </body>
            </html>
          `);
          server.close();
          reject(new Error(error));
          return;
        }

        if (!(code && state)) {
          res.writeHead(400, { "Content-Type": "text/html" });
          res.end(`
            <html>
              <body style="font-family: system-ui; padding: 40px; text-align: center;">
                <h1>Invalid Response</h1>
                <p>Missing code or state parameter.</p>
                <p>You can close this window.</p>
              </body>
            </html>
          `);
          server.close();
          reject(new Error("Missing code or state"));
          return;
        }

        // Validate state to prevent CSRF attacks
        if (state !== expectedState) {
          res.writeHead(400, { "Content-Type": "text/html" });
          res.end(`
            <html>
              <body style="font-family: system-ui; padding: 40px; text-align: center;">
                <h1>Security Error</h1>
                <p>State mismatch - possible CSRF attack.</p>
                <p>You can close this window.</p>
              </body>
            </html>
          `);
          server.close();
          reject(new Error("State mismatch: possible CSRF attack"));
          return;
        }

        res.writeHead(200, { "Content-Type": "text/html" });
        res.end(`
          <html>
            <body style="font-family: system-ui; padding: 40px; text-align: center;">
              <h1>Authorization Successful</h1>
              <p>You can close this window and return to the terminal.</p>
            </body>
          </html>
        `);
        server.close();
        resolve({ code, state });
      }
    });

    // Bind to loopback only for security (prevents network interception)
    server.listen(port, "127.0.0.1");

    // Timeout after 2 minutes
    setTimeout(() => {
      server.close();
      reject(new Error("Authorization timed out"));
    }, 120_000);
  });

export const githubLinkCommand = defineCommand({
  meta: {
    name: "github",
    description: "Link your GitHub account to your Detent team",
  },
  args: {
    team: {
      type: "string",
      description: "Team ID or slug (optional - will prompt if not provided)",
      alias: "t",
    },
  },
  run: async ({ args }) => {
    const repoRoot = await findGitRoot(process.cwd());
    if (!repoRoot) {
      console.error("Not in a git repository.");
      process.exit(1);
    }

    let accessToken: string;
    try {
      accessToken = await getAccessToken(repoRoot);
    } catch {
      console.error("Not logged in. Run `detent auth login` first.");
      process.exit(1);
    }

    // Get user's teams
    console.log("Fetching your teams...");
    const teamsResponse = await getTeams(accessToken).catch((error) => {
      console.error(
        "Failed to fetch teams:",
        error instanceof Error ? error.message : error
      );
      process.exit(1);
    });

    // Select team
    let selectedTeam: Team;

    if (args.team) {
      const found = findTeamByIdOrSlug(teamsResponse.teams, args.team);
      if (!found) {
        console.error(`Team not found: ${args.team}`);
        process.exit(1);
      }
      selectedTeam = found;
    } else {
      const selected = await selectTeam(teamsResponse.teams);
      if (!selected) {
        process.exit(1);
      }
      selectedTeam = selected;
    }

    // Check if already linked
    if (selectedTeam.github_linked) {
      console.log(
        `\nAlready linked to GitHub as @${selectedTeam.github_username}`
      );
      console.log("Run `detent link status` to see details.");
      return;
    }

    console.log(`\nLinking GitHub to team: ${selectedTeam.team_name}`);

    // Start local server for OAuth callback (use 127.0.0.1 for security)
    const port = await findAvailablePort();
    const redirectUri = `http://127.0.0.1:${port}/callback`;

    // Generate PKCE values (OAuth 2.1 requirement for public clients)
    const codeVerifier = generateCodeVerifier();
    const codeChallenge = generateCodeChallenge(codeVerifier);

    // Get authorization URL from API
    let authResponse: AuthorizeResponse;
    try {
      authResponse = await getAuthorizeUrl(
        accessToken,
        selectedTeam.team_id,
        redirectUri,
        codeChallenge
      );
    } catch (error) {
      console.error(
        "Failed to get authorization URL:",
        error instanceof Error ? error.message : error
      );
      process.exit(1);
    }

    // Start local server to receive callback (with state validation)
    const codePromise = waitForOAuthCallback(port, authResponse.state);

    // Open browser
    console.log("\nOpening browser for GitHub authorization...");
    console.log(
      `If browser doesn't open, visit:\n  ${authResponse.authorization_url}\n`
    );
    await openBrowser(authResponse.authorization_url);

    console.log("Waiting for authorization...");

    // Wait for callback
    let callbackData: OAuthCallbackResult;
    try {
      callbackData = await codePromise;
    } catch (error) {
      console.error(
        "\nAuthorization failed:",
        error instanceof Error ? error.message : error
      );
      process.exit(1);
    }

    // Exchange code via API (with PKCE code_verifier)
    console.log("Completing authorization...");
    try {
      const result = await submitCallback(
        accessToken,
        callbackData.code,
        callbackData.state,
        selectedTeam.team_id,
        codeVerifier
      );

      console.log(
        `\nSuccessfully linked GitHub account: @${result.github_username}`
      );
      console.log(
        "\nYour GitHub identity is now associated with your Detent team membership."
      );
      console.log("Run `detent link status` to see details.");
    } catch (error) {
      console.error(
        "\nFailed to complete authorization:",
        error instanceof Error ? error.message : error
      );
      process.exit(1);
    }
  },
});
