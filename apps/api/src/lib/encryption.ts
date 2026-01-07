/**
 * Token encryption utilities using Web Crypto API (AES-256-GCM)
 * Compatible with Cloudflare Workers runtime
 */

const base64ToBuffer = (base64: string): Uint8Array => {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
};

const bufferToBase64 = (buffer: ArrayBuffer | Uint8Array): string => {
  const bytes = buffer instanceof Uint8Array ? buffer : new Uint8Array(buffer);
  // Use Array.from + join instead of string concatenation (O(n) vs O(n²))
  return btoa(Array.from(bytes, (b) => String.fromCharCode(b)).join(""));
};

/**
 * Encrypts a plaintext string using AES-256-GCM
 * @param plaintext - The string to encrypt
 * @param keyBase64 - Base64-encoded 256-bit key (from ENCRYPTION_KEY env var)
 * @returns Encrypted string in format: iv:ciphertext (both base64 encoded)
 */
export const encryptToken = async (
  plaintext: string,
  keyBase64: string
): Promise<string> => {
  const keyBuffer = base64ToBuffer(keyBase64);
  if (keyBuffer.length !== 32) {
    throw new Error("Encryption key must be 256 bits (32 bytes)");
  }

  const key = await crypto.subtle.importKey(
    "raw",
    keyBuffer,
    { name: "AES-GCM" },
    false,
    ["encrypt"]
  );

  const iv = crypto.getRandomValues(new Uint8Array(12));
  const encoded = new TextEncoder().encode(plaintext);

  const ciphertext = await crypto.subtle.encrypt(
    { name: "AES-GCM", iv },
    key,
    encoded
  );

  return `${bufferToBase64(iv)}:${bufferToBase64(ciphertext)}`;
};

/**
 * Decrypts an encrypted token string
 * @param encrypted - Encrypted string in format: iv:ciphertext
 * @param keyBase64 - Base64-encoded 256-bit key (same key used for encryption)
 * @returns Decrypted plaintext string
 */
export const decryptToken = async (
  encrypted: string,
  keyBase64: string
): Promise<string> => {
  const parts = encrypted.split(":");
  if (parts.length !== 2 || !parts[0] || !parts[1]) {
    throw new Error("Invalid encrypted token format");
  }

  const ivBase64 = parts[0];
  const ciphertextBase64 = parts[1];

  let iv: Uint8Array;
  let ciphertext: Uint8Array;
  try {
    iv = base64ToBuffer(ivBase64);
    ciphertext = base64ToBuffer(ciphertextBase64);
  } catch {
    throw new Error("Invalid base64 encoding in encrypted token");
  }

  const keyBuffer = base64ToBuffer(keyBase64);
  if (keyBuffer.length !== 32) {
    throw new Error("Encryption key must be 256 bits (32 bytes)");
  }

  const key = await crypto.subtle.importKey(
    "raw",
    keyBuffer,
    { name: "AES-GCM" },
    false,
    ["decrypt"]
  );

  const decrypted = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv },
    key,
    ciphertext
  );

  return new TextDecoder().decode(decrypted);
};

/**
 * Generates a random 256-bit encryption key
 * Use this to create the ENCRYPTION_KEY env var value
 * @returns Base64-encoded 32-byte key
 */
export const generateEncryptionKey = (): string => {
  const key = crypto.getRandomValues(new Uint8Array(32));
  return bufferToBase64(key);
};
