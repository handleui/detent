/**
 * Global config management
 * Ported from Go: apps/go-cli/internal/persistence/config.go
 *
 * Handles loading and saving the global config file at ~/.detent/detent.json
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import type { Config, GlobalConfig } from "./types.js";

// ============================================================================
// Constants
// ============================================================================

const DETENT_DIR_NAME = ".detent";
const GLOBAL_CONFIG_FILE = "detent.json";
const SCHEMA_URL = "./schema.json";

// Defaults
const DEFAULT_MODEL = "claude-sonnet-4-5";
const DEFAULT_BUDGET_PER_RUN_USD = 1.0;
const DEFAULT_TIMEOUT_MINS = 10;

// Constraints
const MIN_TIMEOUT_MINS = 1;
const MAX_TIMEOUT_MINS = 60;
const MIN_BUDGET_USD = 0.0;
const MAX_BUDGET_USD = 100.0;
const MAX_BUDGET_MONTHLY_USD = 1000.0;
const MODEL_PREFIX = "claude-";

// ============================================================================
// Path Helpers
// ============================================================================

/**
 * Gets the detent directory path (~/.detent)
 */
export const getDetentDir = (): string => {
  const override = process.env.DETENT_HOME;
  if (override) {
    return override;
  }
  return join(homedir(), DETENT_DIR_NAME);
};

/**
 * Gets the path to the global config file
 */
export const getConfigPath = (): string => {
  return join(getDetentDir(), GLOBAL_CONFIG_FILE);
};

// ============================================================================
// Config Loading
// ============================================================================

/**
 * Loads the global config, returning the resolved Config
 */
export const loadConfig = (): Config => {
  const global = loadGlobalConfig();
  return mergeConfig(global);
};

/**
 * Loads the raw global config from disk
 */
export const loadGlobalConfig = (): GlobalConfig => {
  const configPath = getConfigPath();

  if (!existsSync(configPath)) {
    return {};
  }

  try {
    const data = readFileSync(configPath, "utf-8");
    if (!data.trim()) {
      return {};
    }
    return JSON.parse(data) as GlobalConfig;
  } catch {
    return {};
  }
};

/**
 * Merges global config with defaults
 */
const mergeConfig = (global: GlobalConfig): Config => {
  const config: Config = {
    apiKey: "",
    model: DEFAULT_MODEL,
    budgetPerRunUsd: DEFAULT_BUDGET_PER_RUN_USD,
    budgetMonthlyUsd: 0, // 0 means unlimited
    timeoutMins: DEFAULT_TIMEOUT_MINS,
  };

  // Apply global config
  if (global.apiKey) {
    config.apiKey = global.apiKey;
  }

  if (global.model) {
    if (global.model.startsWith(MODEL_PREFIX)) {
      config.model = global.model;
    } else {
      console.error(
        `warning: ignoring invalid model "${global.model}" (must start with "${MODEL_PREFIX}")`
      );
    }
  }

  if (global.budgetPerRunUsd !== undefined) {
    config.budgetPerRunUsd = clampBudget(global.budgetPerRunUsd);
  }

  if (global.budgetMonthlyUsd !== undefined) {
    config.budgetMonthlyUsd = clampMonthlyBudget(global.budgetMonthlyUsd);
  }

  if (global.timeoutMins !== undefined) {
    config.timeoutMins = clampTimeout(global.timeoutMins);
  }

  // Environment variable overrides everything for API key
  const envKey = process.env.ANTHROPIC_API_KEY;
  if (envKey) {
    config.apiKey = envKey;
  }

  return config;
};

// ============================================================================
// Config Saving
// ============================================================================

/**
 * Saves the global config to disk
 */
export const saveConfig = (config: GlobalConfig): void => {
  const dir = getDetentDir();

  // Ensure directory exists
  if (!existsSync(dir)) {
    mkdirSync(dir, { mode: 0o700, recursive: true });
  }

  // Add schema reference
  const configWithSchema = {
    $schema: SCHEMA_URL,
    ...config,
  };

  const data = `${JSON.stringify(configWithSchema, null, 2)}\n`;
  const configPath = join(dir, GLOBAL_CONFIG_FILE);

  writeFileSync(configPath, data, { mode: 0o600 });
};

// ============================================================================
// Clamping Helpers
// ============================================================================

const clampBudget = (value: number): number => {
  if (value < MIN_BUDGET_USD) {
    return MIN_BUDGET_USD;
  }
  if (value > MAX_BUDGET_USD) {
    return MAX_BUDGET_USD;
  }
  return value;
};

const clampMonthlyBudget = (value: number): number => {
  if (value < 0) {
    return 0;
  }
  if (value > MAX_BUDGET_MONTHLY_USD) {
    return MAX_BUDGET_MONTHLY_USD;
  }
  return value;
};

const clampTimeout = (value: number): number => {
  if (value < MIN_TIMEOUT_MINS) {
    return MIN_TIMEOUT_MINS;
  }
  if (value > MAX_TIMEOUT_MINS) {
    return MAX_TIMEOUT_MINS;
  }
  return value;
};

// ============================================================================
// Trust Helpers
// ============================================================================

/**
 * Checks if a repository is trusted by its first commit SHA
 */
export const isTrustedRepo = (
  config: GlobalConfig,
  firstCommitSha: string
): boolean => {
  return config.trustedRepos?.[firstCommitSha] !== undefined;
};

/**
 * Marks a repository as trusted
 */
export const trustRepo = (
  config: GlobalConfig,
  firstCommitSha: string,
  remoteUrl?: string
): GlobalConfig => {
  return {
    ...config,
    trustedRepos: {
      ...config.trustedRepos,
      [firstCommitSha]: {
        remoteUrl,
        trustedAt: new Date(),
      },
    },
  };
};

// ============================================================================
// Command Helpers
// ============================================================================

/**
 * Gets the allowed commands for a repo by its first commit SHA
 */
export const getAllowedCommands = (
  config: GlobalConfig,
  repoSha: string
): string[] => {
  return config.allowedCommands?.[repoSha]?.commands ?? [];
};

/**
 * Checks if a command matches the repo's allowlist (supports wildcards)
 */
export const matchesCommand = (
  config: GlobalConfig,
  repoSha: string,
  cmd: string
): boolean => {
  const commands = getAllowedCommands(config, repoSha);

  for (const pattern of commands) {
    if (cmd === pattern) {
      return true;
    }

    // Wildcard pattern: "bun run *" matches "bun run test"
    if (pattern.endsWith(" *")) {
      const prefix = pattern.slice(0, -1); // Remove "*"
      if (cmd.startsWith(prefix)) {
        const suffix = cmd.slice(prefix.length);

        // Reject dangerous patterns
        if (containsDangerousPattern(suffix)) {
          continue;
        }

        // Reject if suffix contains spaces (wildcard should match single argument)
        if (suffix.trim().includes(" ")) {
          continue;
        }

        return true;
      }
    }
  }

  return false;
};

const DANGEROUS_PATTERNS = [
  ";",
  "&&",
  "||",
  "|",
  ">",
  "<",
  ">>",
  "<<",
  "$(",
  "`",
  "${",
  "\\",
  "\n",
  "\r",
  "\0",
];

const containsDangerousPattern = (s: string): boolean => {
  return DANGEROUS_PATTERNS.some((pattern) => s.includes(pattern));
};

// ============================================================================
// Display Helpers
// ============================================================================

/**
 * Masks an API key for safe display
 */
export const maskApiKey = (key: string): string => {
  if (!key) {
    return "";
  }
  if (key.length <= 4) {
    return "****";
  }
  return `****${key.slice(-4)}`;
};

/**
 * Formats a budget value for display
 */
export const formatBudget = (usd: number): string => {
  if (usd === 0) {
    return "unlimited";
  }
  return `$${usd.toFixed(2)}`;
};
