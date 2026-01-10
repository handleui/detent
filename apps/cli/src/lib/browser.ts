/**
 * Cross-platform browser opener
 */
export const openBrowser = async (url: string): Promise<void> => {
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
