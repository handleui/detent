/**
 * Detent CLI Brand Colors
 *
 * Minimal palette (4 colors):
 * - brand: Bright green (#00FF00) - Logo, active indicator
 * - text: White - Primary content
 * - muted: Gray - Hints, inactive
 * - error: Red - Errors only
 */

export const colors = {
  brand: "#17DB4E", // Electric green (less harsh than pure #00FF00)
  text: "white",
  muted: "gray",
  error: "red",
} as const;

export type Color = (typeof colors)[keyof typeof colors];

/**
 * Converts a hex color to ANSI escape code for true color (24-bit) terminals.
 */
export const hexToAnsi = (hex: string): string => {
  const cleaned = hex.replace("#", "");
  const r = Number.parseInt(cleaned.slice(0, 2), 16);
  const g = Number.parseInt(cleaned.slice(2, 4), 16);
  const b = Number.parseInt(cleaned.slice(4, 6), 16);
  return `\x1b[38;2;${r};${g};${b}m`;
};

export const ANSI_RESET = "\x1b[0m";
