/**
 * Shared error handlers for CLI commands
 */

export const handleGitHubOrgError = (error: unknown): never => {
  const message = error instanceof Error ? error.message : String(error);

  if (message.includes("GitHub account not connected")) {
    console.error("GitHub account not connected.");
    console.error(
      "Please authenticate with GitHub: run `detent auth login --force`"
    );
  } else if (message.includes("authorization expired")) {
    console.error("GitHub authorization expired.");
    console.error("Please re-authenticate: run `detent auth login --force`");
  } else {
    console.error("Failed to fetch GitHub organizations:", message);
  }
  process.exit(1);
};
