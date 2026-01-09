/**
 * Generate a secure random token for invitation links
 * Uses 32 bytes (256 bits) of randomness, base64url encoded
 */
export const generateSecureToken = (): string => {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);

  // Base64url encode (URL-safe, no padding)
  return btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=/g, "");
};
