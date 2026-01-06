/**
 * Initialize detent in a repository
 *
 * Creates .detent/ directory and config file.
 * Required for heal command, optional for check/config.
 */

import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { findGitRoot, validateGitRepository } from "@detent/git";
import { defineCommand } from "citty";
import { Box, render, Text, useApp, useInput } from "ink";
import { useState } from "react";
import {
  ensureRepoDetentDir,
  getRepoConfigPath,
  getRepoDetentDir,
  isRepoInitialized,
  loadRepoConfig,
  saveRepoConfig,
  validateApiKey,
} from "../lib/config.js";
import { Header } from "../tui/components/index.js";
import { shouldUseTUI } from "../tui/render.js";
import { colors } from "../tui/styles.js";

// ============================================================================
// Gitignore Helper
// ============================================================================

const GITIGNORE_ENTRY = ".detent/";

const addToGitignore = (repoRoot: string): boolean => {
  const gitignorePath = join(repoRoot, ".gitignore");

  if (existsSync(gitignorePath)) {
    const content = readFileSync(gitignorePath, "utf-8");
    if (content.includes(GITIGNORE_ENTRY)) {
      return false; // Already present
    }

    // Append with newline
    const newContent = content.endsWith("\n")
      ? `${content}${GITIGNORE_ENTRY}\n`
      : `${content}\n${GITIGNORE_ENTRY}\n`;
    writeFileSync(gitignorePath, newContent);
  } else {
    writeFileSync(gitignorePath, `${GITIGNORE_ENTRY}\n`);
  }
  return true;
};

// ============================================================================
// Verbose Mode (Non-TUI)
// ============================================================================

const runVerboseInit = (
  repoRoot: string,
  apiKey: string | undefined,
  force: boolean
): void => {
  console.log("Detent initialization\n");

  // Check if already initialized
  if (isRepoInitialized(repoRoot) && !force) {
    console.log(`Already initialized at ${getRepoDetentDir(repoRoot)}`);
    console.log("Use --force to reinitialize.\n");
    return;
  }

  // Detect API key from environment
  const envKey = process.env.ANTHROPIC_API_KEY;
  const finalKey = apiKey || envKey || "";

  if (finalKey) {
    const validation = validateApiKey(finalKey);
    if (!validation.valid) {
      console.error(`Error: ${validation.error}`);
      process.exit(1);
    }
    console.log(
      envKey && !apiKey
        ? "Using API key from ANTHROPIC_API_KEY environment variable"
        : "Using provided API key"
    );
  } else {
    console.log("No API key provided (heal command will require one)");
  }

  // Create .detent/ directory
  ensureRepoDetentDir(repoRoot);
  console.log(`Created ${getRepoDetentDir(repoRoot)}/`);

  // Save config
  const existingConfig = loadRepoConfig(repoRoot);
  saveRepoConfig(
    {
      ...existingConfig,
      ...(finalKey ? { apiKey: finalKey } : {}),
    },
    repoRoot
  );
  console.log(`Created ${getRepoConfigPath(repoRoot)}`);

  // Add to .gitignore
  const added = addToGitignore(repoRoot);
  if (added) {
    console.log("Added .detent/ to .gitignore");
  } else {
    console.log(".detent/ already in .gitignore");
  }

  console.log("\nInitialization complete!");
  console.log("Run 'detent mock' to scan for errors.");
};

// ============================================================================
// TUI Mode
// ============================================================================

type InitStep = "already-init" | "api-key" | "creating" | "done" | "error";

interface InitTUIProps {
  repoRoot: string;
  force: boolean;
}

// Compute initial state synchronously to avoid flash of "checking" state
const computeInitialState = (
  repoRoot: string,
  force: boolean
): { step: InitStep; apiKey: string; keySource: "env" | "input" | "none" } => {
  if (isRepoInitialized(repoRoot) && !force) {
    return { step: "already-init", apiKey: "", keySource: "none" };
  }

  const envKey = process.env.ANTHROPIC_API_KEY;
  if (envKey) {
    const validation = validateApiKey(envKey);
    if (validation.valid) {
      return { step: "creating", apiKey: envKey, keySource: "env" };
    }
  }

  return { step: "api-key", apiKey: "", keySource: "none" };
};

const InitTUI = ({ repoRoot, force }: InitTUIProps): JSX.Element => {
  const { exit } = useApp();
  const initial = computeInitialState(repoRoot, force);
  const [step, setStep] = useState<InitStep>(initial.step);
  const [apiKey, setApiKey] = useState(initial.apiKey);
  const [error, setError] = useState<string | null>(null);
  const [keySource, setKeySource] = useState<"env" | "input" | "none">(
    initial.keySource
  );
  const [configCreated, setConfigCreated] = useState(false);

  const createConfig = (key: string): void => {
    if (configCreated) {
      return;
    }
    try {
      ensureRepoDetentDir(repoRoot);
      const existingConfig = loadRepoConfig(repoRoot);
      saveRepoConfig(
        {
          ...existingConfig,
          ...(key ? { apiKey: key } : {}),
        },
        repoRoot
      );
      addToGitignore(repoRoot);
      setConfigCreated(true);
      setStep("done");
      // Auto-exit after brief display
      setTimeout(() => exit(), 500);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
      setStep("error");
      setTimeout(() => exit(), 1000);
    }
  };

  // Handle auto-config creation when env key is detected
  if (step === "creating" && keySource === "env" && !configCreated) {
    createConfig(apiKey);
  }

  // Auto-exit on already-init
  if (step === "already-init") {
    setTimeout(() => exit(), 1000);
  }

  const handleExitSteps = (
    input: string,
    key: { return?: boolean }
  ): boolean => {
    const isExitStep =
      step === "already-init" || step === "done" || step === "error";
    if (!isExitStep) {
      return false;
    }
    // Exit on any input (or auto-exits via setTimeout)
    if (key.return || input === "q" || input) {
      exit();
    }
    return true;
  };

  const handleApiKeySubmit = (): void => {
    if (apiKey) {
      const validation = validateApiKey(apiKey);
      if (!validation.valid) {
        setError(validation.error ?? "Invalid API key");
        return;
      }
      setKeySource("input");
      setStep("creating");
      createConfig(apiKey);
    } else {
      setKeySource("none");
      setStep("creating");
      createConfig("");
    }
  };

  const handleApiKeyInput = (
    input: string,
    key: {
      return?: boolean;
      backspace?: boolean;
      delete?: boolean;
      ctrl?: boolean;
      meta?: boolean;
    }
  ): void => {
    if (key.return) {
      handleApiKeySubmit();
      return;
    }

    // Ctrl+U clears the input (standard Unix line-kill)
    if (key.ctrl && input === "u") {
      setApiKey("");
      setError(null);
      return;
    }

    if (key.backspace || key.delete) {
      setApiKey((prev) => prev.slice(0, -1));
      setError(null);
      return;
    }

    if (input && !key.ctrl && !key.meta) {
      setApiKey((prev) => prev + input);
      setError(null);
    }
  };

  useInput((input, key) => {
    if (handleExitSteps(input, key)) {
      return;
    }
    if (step === "api-key") {
      handleApiKeyInput(input, key);
    }
  });

  return (
    <Box flexDirection="column" paddingBottom={1} paddingX={1}>
      <Header command="init" />

      {step === "already-init" && (
        <Text color={colors.warn}>
          Already initialized. Use --force to reinitialize.
        </Text>
      )}

      {step === "api-key" && (
        <Box flexDirection="column">
          <Box>
            <Text>API Key: </Text>
            {apiKey ? (
              <Text color={colors.brand}>{apiKey}</Text>
            ) : (
              <Text color={colors.muted}>_</Text>
            )}
          </Box>
          {error && <Text color={colors.error}>{error}</Text>}
          <Text> </Text>
          <Text color={colors.muted}>
            Get your key at console.anthropic.com/settings/keys • ctrl + u to
            clear • enter to skip
          </Text>
        </Box>
      )}

      {step === "creating" && <Text color={colors.muted}>...</Text>}

      {step === "done" && (
        <Text>
          <Text color={colors.brand}>✓</Text>
          <Text> Created .detent</Text>
          <Text color={colors.muted}>
            {keySource === "env" && " • using ANTHROPIC_API_KEY"}
            {keySource === "input" && " • API key saved"}
            {keySource === "none" && " • API key required for 'dt heal'"}
          </Text>
        </Text>
      )}

      {step === "error" && <Text color={colors.error}>✗ {error}</Text>}
    </Box>
  );
};

// ============================================================================
// Command Definition
// ============================================================================

export const initCommand = defineCommand({
  meta: {
    name: "init",
    description: "Initialize detent in the current repository",
  },
  args: {
    force: {
      type: "boolean",
      description: "Reinitialize even if already configured",
      alias: "f",
      default: false,
    },
    "api-key": {
      type: "string",
      description: "API key (or use ANTHROPIC_API_KEY env var)",
    },
  },
  run: async ({ args }) => {
    const cwd = process.cwd();

    // Validate git repo
    try {
      await validateGitRepository(cwd);
    } catch {
      console.error("Error: Not a git repository.");
      console.error("Run 'git init' first, or navigate to an existing repo.");
      process.exit(1);
    }

    const repoRoot = (await findGitRoot(cwd)) ?? cwd;
    const force = args.force as boolean;
    const apiKey = args["api-key"] as string | undefined;

    // Use TUI mode if available
    if (shouldUseTUI()) {
      const { waitUntilExit } = render(
        <InitTUI force={force} repoRoot={repoRoot} />,
        { exitOnCtrlC: true }
      );
      await waitUntilExit();
    } else {
      runVerboseInit(repoRoot, apiKey, force);
    }
  },
});
