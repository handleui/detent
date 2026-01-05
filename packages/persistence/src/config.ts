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

// API Key validation patterns
const API_KEY_PREFIXES = ["sk-ant-"] as const;
const API_KEY_MIN_LENGTH = 20;
const API_KEY_MAX_LENGTH = 200;

// Allowed models (canonical list - only 4.5 generation)
const ALLOWED_MODELS = [
  "claude-opus-4-5",
  "claude-sonnet-4-5",
  "claude-haiku-4-5",
] as const;

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

// ============================================================================
// Validation Helpers
// ============================================================================

export interface ValidationResult {
  valid: boolean;
  error?: string;
}

/**
 * Validates an API key format.
 * Keys must start with a known prefix and be within length bounds.
 */
export const validateApiKey = (key: string): ValidationResult => {
  if (!key || key.trim() === "") {
    return { valid: false, error: "API key is required" };
  }

  const trimmed = key.trim();

  if (trimmed.length < API_KEY_MIN_LENGTH) {
    return {
      valid: false,
      error:
        "API key is too short. Expected format: sk-ant-api03-... (get yours from console.anthropic.com)",
    };
  }

  if (trimmed.length > API_KEY_MAX_LENGTH) {
    return {
      valid: false,
      error:
        "API key is too long. Expected format: sk-ant-api03-... (get yours from console.anthropic.com)",
    };
  }

  const hasValidPrefix = API_KEY_PREFIXES.some((prefix) =>
    trimmed.startsWith(prefix)
  );
  if (!hasValidPrefix) {
    return {
      valid: false,
      error: `Invalid API key format. Must start with ${API_KEY_PREFIXES.join(" or ")} (e.g., sk-ant-api03-...). Get your key from console.anthropic.com`,
    };
  }

  return { valid: true };
};

/**
 * Validates a model name.
 * Must be in the allowed models list or start with claude- prefix.
 */
export const validateModel = (model: string): ValidationResult => {
  if (!model || model.trim() === "") {
    return { valid: false, error: "Model name is required" };
  }

  const trimmed = model.trim();

  if (ALLOWED_MODELS.includes(trimmed as (typeof ALLOWED_MODELS)[number])) {
    return { valid: true };
  }

  if (!trimmed.startsWith(MODEL_PREFIX)) {
    return {
      valid: false,
      error: `Model must start with "${MODEL_PREFIX}" or be one of: ${ALLOWED_MODELS.join(", ")}`,
    };
  }

  return { valid: true };
};

/**
 * Validates a budget value.
 */
export const validateBudgetPerRun = (value: number): ValidationResult => {
  if (Number.isNaN(value)) {
    return { valid: false, error: "Budget must be a number" };
  }
  if (value < MIN_BUDGET_USD) {
    return { valid: false, error: "Budget cannot be negative" };
  }
  if (value > MAX_BUDGET_USD) {
    return {
      valid: false,
      error: `Budget cannot exceed $${MAX_BUDGET_USD}`,
    };
  }
  return { valid: true };
};

/**
 * Validates a monthly budget value.
 */
export const validateBudgetMonthly = (value: number): ValidationResult => {
  if (Number.isNaN(value)) {
    return { valid: false, error: "Monthly budget must be a number" };
  }
  if (value < 0) {
    return { valid: false, error: "Monthly budget cannot be negative" };
  }
  if (value > MAX_BUDGET_MONTHLY_USD) {
    return {
      valid: false,
      error: `Monthly budget cannot exceed $${MAX_BUDGET_MONTHLY_USD}`,
    };
  }
  return { valid: true };
};

/**
 * Validates a timeout value in minutes.
 */
export const validateTimeout = (value: number): ValidationResult => {
  if (Number.isNaN(value)) {
    return { valid: false, error: "Timeout must be a number" };
  }
  if (value < 0) {
    return { valid: false, error: "Timeout cannot be negative" };
  }
  if (value > 0 && value < MIN_TIMEOUT_MINS) {
    return {
      valid: false,
      error: `Timeout must be at least ${MIN_TIMEOUT_MINS} minute(s)`,
    };
  }
  if (value > MAX_TIMEOUT_MINS) {
    return {
      valid: false,
      error: `Timeout cannot exceed ${MAX_TIMEOUT_MINS} minutes`,
    };
  }
  return { valid: true };
};

/**
 * Gets the list of allowed models
 */
export const getAllowedModels = (): readonly string[] => ALLOWED_MODELS;
