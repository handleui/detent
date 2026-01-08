/**
 * Input validation utilities for API routes
 *
 * Provides validation for slugs, handles, and other user inputs
 * to prevent injection attacks and ensure data consistency.
 */

// Valid slug/handle pattern: lowercase alphanumeric with hyphens, no leading/trailing hyphens
// Allows forward slash for provider-prefixed slugs (e.g., gh/org-name)
const SLUG_PATTERN = /^[a-z0-9]+(?:[-/][a-z0-9]+)*$/;

// Handle pattern: same as slug but without forward slashes
const HANDLE_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

// UUID v4 pattern
const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

// Provider values
const VALID_PROVIDERS = ["github", "gitlab"] as const;

interface ValidationResult {
  valid: boolean;
  error?: string;
}

/**
 * Validate a slug (e.g., organization slug like "gh/my-org")
 * - Must be 1-255 characters
 * - Lowercase alphanumeric with hyphens and optional forward slash
 * - No leading/trailing hyphens or slashes
 * - No consecutive hyphens or slashes
 */
export const validateSlug = (
  slug: string,
  fieldName = "slug"
): ValidationResult => {
  if (!slug || typeof slug !== "string") {
    return { valid: false, error: `${fieldName} is required` };
  }

  const trimmed = slug.trim();

  if (trimmed.length === 0) {
    return { valid: false, error: `${fieldName} cannot be empty` };
  }

  if (trimmed.length > 255) {
    return {
      valid: false,
      error: `${fieldName} must be 255 characters or less`,
    };
  }

  if (!SLUG_PATTERN.test(trimmed)) {
    return {
      valid: false,
      error: `${fieldName} must contain only lowercase letters, numbers, hyphens, and forward slashes`,
    };
  }

  return { valid: true };
};

/**
 * Validate a handle (e.g., project handle like "my-project")
 * - Must be 1-255 characters
 * - Lowercase alphanumeric with hyphens only (no slashes)
 * - No leading/trailing hyphens
 * - No consecutive hyphens
 */
export const validateHandle = (
  handle: string,
  fieldName = "handle"
): ValidationResult => {
  if (!handle || typeof handle !== "string") {
    return { valid: false, error: `${fieldName} is required` };
  }

  const trimmed = handle.trim().toLowerCase();

  if (trimmed.length === 0) {
    return { valid: false, error: `${fieldName} cannot be empty` };
  }

  if (trimmed.length > 255) {
    return {
      valid: false,
      error: `${fieldName} must be 255 characters or less`,
    };
  }

  if (!HANDLE_PATTERN.test(trimmed)) {
    return {
      valid: false,
      error: `${fieldName} must contain only lowercase letters, numbers, and hyphens`,
    };
  }

  return { valid: true };
};

/**
 * Validate a UUID
 */
export const validateUUID = (
  id: string,
  fieldName = "id"
): ValidationResult => {
  if (!id || typeof id !== "string") {
    return { valid: false, error: `${fieldName} is required` };
  }

  if (!UUID_PATTERN.test(id)) {
    return { valid: false, error: `${fieldName} must be a valid UUID` };
  }

  return { valid: true };
};

/**
 * Validate a provider value
 */
export const validateProvider = (
  provider: string,
  fieldName = "provider"
): ValidationResult => {
  if (!provider || typeof provider !== "string") {
    return { valid: false, error: `${fieldName} is required` };
  }

  if (!VALID_PROVIDERS.includes(provider as (typeof VALID_PROVIDERS)[number])) {
    return {
      valid: false,
      error: `${fieldName} must be one of: ${VALID_PROVIDERS.join(", ")}`,
    };
  }

  return { valid: true };
};

/**
 * Sanitize a string for use as a handle
 * Converts to lowercase and replaces invalid characters with hyphens
 */
export const sanitizeHandle = (input: string): string => {
  return input
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, "-") // Replace invalid chars with hyphens
    .replace(/-+/g, "-") // Collapse multiple hyphens
    .replace(/^-|-$/g, ""); // Remove leading/trailing hyphens
};
