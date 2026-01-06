/**
 * Config management for Detent CLI
 *
 * Two modes:
 * - Per-repo: .detent/config.json in repository root (preferred)
 * - Global: ~/.detent/detent.json (legacy, for shared resources only)
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

// ============================================================================
// Types
// ============================================================================

/**
 * GlobalConfig is the raw structure that gets persisted to disk
 * Used for both per-repo .detent/config.json and legacy ~/.detent/detent.json
 */
export interface GlobalConfig {
  $schema?: string;
  apiKey?: string;
  model?: string;
  budgetPerRunUsd?: number;
  budgetMonthlyUsd?: number;
  timeoutMins?: number;
}

/**
 * Config is the merged, resolved config used by the application
 */
export interface Config {
  apiKey: string;
  model: string;
  budgetPerRunUsd: number;
  budgetMonthlyUsd: number;
  timeoutMins: number;
}

export interface ValidationResult {
  valid: boolean;
  error?: string;
}

export interface ConfigLoadResult {
  config: GlobalConfig;
  error?: string;
}

// ============================================================================
// Constants
// ============================================================================

const DETENT_DIR_NAME = ".detent";
const REPO_CONFIG_FILE = "config.json";
const GLOBAL_CONFIG_FILE = "detent.json";
const SCHEMA_URL = "./schema.json";

const DEFAULT_MODEL = "claude-sonnet-4-5";
const DEFAULT_BUDGET_PER_RUN_USD = 1.0;
const DEFAULT_TIMEOUT_MINS = 10;

const MIN_TIMEOUT_MINS = 1;
const MAX_TIMEOUT_MINS = 60;
const MIN_BUDGET_USD = 0.0;
const MAX_BUDGET_USD = 100.0;
const MAX_BUDGET_MONTHLY_USD = 1000.0;
const MODEL_PREFIX = "claude-";

const API_KEY_PREFIXES = ["sk-ant-"] as const;
const API_KEY_MIN_LENGTH = 20;
const API_KEY_MAX_LENGTH = 200;

const ALLOWED_MODELS = [
  "claude-opus-4-5",
  "claude-sonnet-4-5",
  "claude-haiku-4-5",
] as const;

const WINDOWS_DRIVE_PATTERN = /^[A-Za-z]:\\/;

// ============================================================================
// Path Helpers
// ============================================================================

const validateOverridePath = (path: string): string | null => {
  if (path.includes("..")) {
    return null;
  }
  if (!(path.startsWith("/") || WINDOWS_DRIVE_PATTERN.test(path))) {
    return null;
  }
  return path;
};

/**
 * Gets the global detent directory path (~/.detent)
 * Used for shared resources like act binary and debug logs
 */
export const getGlobalDetentDir = (): string => {
  const override = process.env.DETENT_HOME;
  if (override) {
    const validated = validateOverridePath(override);
    if (validated) {
      return validated;
    }
  }
  return join(homedir(), DETENT_DIR_NAME);
};

/**
 * @deprecated Use getGlobalDetentDir() instead
 */
export const getDetentDir = getGlobalDetentDir;

/**
 * Gets the per-repo detent directory path (<repo>/.detent)
 */
export const getRepoDetentDir = (repoRoot: string): string => {
  return join(repoRoot, DETENT_DIR_NAME);
};

/**
 * Gets the path to the per-repo config file (<repo>/.detent/config.json)
 */
export const getRepoConfigPath = (repoRoot: string): string => {
  return join(getRepoDetentDir(repoRoot), REPO_CONFIG_FILE);
};

/**
 * Gets the path to the global config file (legacy)
 * @deprecated Use getRepoConfigPath() for per-repo config
 */
export const getConfigPath = (): string => {
  return join(getGlobalDetentDir(), GLOBAL_CONFIG_FILE);
};

/**
 * Creates the .detent/ directory in a repo if it doesn't exist
 */
export const ensureRepoDetentDir = (repoRoot: string): void => {
  const dir = getRepoDetentDir(repoRoot);
  if (!existsSync(dir)) {
    mkdirSync(dir, { mode: 0o700, recursive: true });
  }
};

/**
 * Checks if a repository has been initialized (has config file)
 */
export const isRepoInitialized = (repoRoot: string): boolean => {
  return existsSync(getRepoConfigPath(repoRoot));
};

// ============================================================================
// Config Loading
// ============================================================================

/**
 * Loads the per-repo config from .detent/config.json
 * Returns empty config for missing files, warns for corrupted/inaccessible files.
 */
export const loadRepoConfig = (repoRoot: string): GlobalConfig => {
  const result = loadRepoConfigSafe(repoRoot);
  if (result.error) {
    console.error(`warning: ${result.error}`);
  }
  return result.config;
};

/**
 * Loads config with detailed error information.
 * Use this when you need to distinguish between "not found" and "corrupted".
 */
export const loadRepoConfigSafe = (repoRoot: string): ConfigLoadResult => {
  const configPath = getRepoConfigPath(repoRoot);

  if (!existsSync(configPath)) {
    return { config: {} };
  }

  try {
    const data = readFileSync(configPath, "utf-8");
    if (!data.trim()) {
      return { config: {} };
    }
    return { config: JSON.parse(data) as GlobalConfig };
  } catch (err) {
    const error = err as NodeJS.ErrnoException;

    if (error.code === "EACCES") {
      return {
        config: {},
        error: `cannot read config at ${configPath}: permission denied`,
      };
    }

    if (error.code === "EISDIR") {
      return {
        config: {},
        error: `config path is a directory: ${configPath}`,
      };
    }

    if (error instanceof SyntaxError) {
      return {
        config: {},
        error: `config file is corrupted: ${configPath} (invalid JSON)`,
      };
    }

    return {
      config: {},
      error: `failed to load config: ${error.message}`,
    };
  }
};

/**
 * Loads the config from per-repo .detent/config.json
 * @param repoRoot - Repository root path (required)
 */
export const loadConfig = (repoRoot: string): Config => {
  const raw = loadRepoConfig(repoRoot);
  return mergeConfig(raw);
};

// ============================================================================
// Config Merging
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

const mergeConfig = (global: GlobalConfig): Config => {
  const config: Config = {
    apiKey: "",
    model: DEFAULT_MODEL,
    budgetPerRunUsd: DEFAULT_BUDGET_PER_RUN_USD,
    budgetMonthlyUsd: 0,
    timeoutMins: DEFAULT_TIMEOUT_MINS,
  };

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
 * Saves config to disk
 * @param config - Config object to save
 * @param repoRoot - If provided, saves to per-repo .detent/config.json
 */
export const saveConfig = (config: GlobalConfig, repoRoot?: string): void => {
  const dir = repoRoot ? getRepoDetentDir(repoRoot) : getGlobalDetentDir();
  const filename = repoRoot ? REPO_CONFIG_FILE : GLOBAL_CONFIG_FILE;

  if (!existsSync(dir)) {
    mkdirSync(dir, { mode: 0o700, recursive: true });
  }

  const configWithSchema = {
    $schema: SCHEMA_URL,
    ...config,
  };

  const data = `${JSON.stringify(configWithSchema, null, 2)}\n`;
  const configPath = join(dir, filename);

  writeFileSync(configPath, data, { mode: 0o600 });
};

/**
 * Saves config to per-repo .detent/config.json
 */
export const saveRepoConfig = (
  config: GlobalConfig,
  repoRoot: string
): void => {
  saveConfig(config, repoRoot);
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
