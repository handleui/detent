/**
 * Formats an unknown error into a string message.
 * Handles Error instances, objects with message property, and fallback to String().
 *
 * @param error - The error to format
 * @returns Formatted error message
 */
export const formatError = (error: unknown): string => {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
};

const ERROR_PREFIX_REGEX = /^Error:\s*/i;
const MAX_ERROR_LENGTH = 100;

/**
 * Formats an error message for TUI display.
 * Makes error messages concise and readable:
 * - Strips "Error:" prefix if present
 * - Keeps first line only if multi-line
 * - Truncates to ~100 chars for TUI display
 *
 * @param message - The error message to format
 * @returns Formatted error message for TUI
 */
export const formatErrorForTUI = (message: string): string => {
  let formatted = message;

  // Strip "Error:" prefix if present
  formatted = formatted.replace(ERROR_PREFIX_REGEX, "");

  // Keep first line only if multi-line
  const firstLine = formatted.split("\n")[0];
  if (firstLine) {
    formatted = firstLine;
  }

  // Truncate to ~100 chars for TUI display
  if (formatted.length > MAX_ERROR_LENGTH) {
    formatted = `${formatted.slice(0, MAX_ERROR_LENGTH - 3)}...`;
  }

  return formatted.trim();
};
