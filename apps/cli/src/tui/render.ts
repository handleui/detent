/**
 * Determines if the CLI should use interactive TUI mode.
 *
 * TUI is enabled by default when running in a TTY.
 * AI agents should use programmatic commands (config get/set) instead.
 */
export const shouldUseTUI = (): boolean => {
  // Explicit override via flag
  if (process.argv.includes("--no-tui")) {
    return false;
  }

  // TUI requires a TTY for interactive input/output
  return Boolean(process.stdout.isTTY && process.stdin.isTTY);
};
