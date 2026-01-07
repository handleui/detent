/**
 * Organization create command
 *
 * Opens browser to install the Detent GitHub App, which creates the organization
 * and associated projects automatically via webhooks.
 */

import { defineCommand } from "citty";

const GITHUB_APP_URL =
  process.env.DETENT_GITHUB_APP_URL ??
  "https://github.com/apps/detent-apps/installations/new";

const openBrowser = async (url: string): Promise<void> => {
  const { platform } = process;
  const { exec } = await import("node:child_process");
  const { promisify } = await import("node:util");
  const execAsync = promisify(exec);

  const commands: Record<string, string> = {
    darwin: `open "${url}"`,
    win32: `start "" "${url}"`,
    linux: `xdg-open "${url}"`,
  };

  const command = commands[platform];
  if (!command) {
    throw new Error(`Unsupported platform: ${platform}`);
  }

  await execAsync(command);
};

export const createCommand = defineCommand({
  meta: {
    name: "create",
    description: "Create a new organization by installing the GitHub App",
  },
  run: async () => {
    console.log("Opening browser to install the Detent GitHub App...\n");
    console.log(
      "After installation, your organization and repositories will be set up automatically.\n"
    );
    console.log(`If browser doesn't open, visit:\n  ${GITHUB_APP_URL}\n`);

    try {
      await openBrowser(GITHUB_APP_URL);
    } catch (error) {
      console.error(
        "Failed to open browser:",
        error instanceof Error ? error.message : error
      );
      console.error(`\nPlease manually visit: ${GITHUB_APP_URL}`);
    }
  },
});
