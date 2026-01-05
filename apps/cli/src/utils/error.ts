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
