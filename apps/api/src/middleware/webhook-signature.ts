import type { Context, Next } from "hono";
import { HTTPException } from "hono/http-exception";

// Verify GitHub webhook signature (X-Hub-Signature-256)
// Uses timing-safe comparison to prevent timing attacks

export const webhookSignatureMiddleware = async (c: Context, next: Next) => {
  const signature = c.req.header("X-Hub-Signature-256");
  const secret = c.env.GITHUB_WEBHOOK_SECRET;

  if (!signature) {
    throw new HTTPException(401, {
      message: "Missing X-Hub-Signature-256 header",
    });
  }

  if (!secret) {
    throw new HTTPException(500, {
      message: "GITHUB_WEBHOOK_SECRET not configured",
    });
  }

  // Get raw body for signature verification
  const rawBody = await c.req.text();

  // Compute expected signature
  const encoder = new TextEncoder();
  const key = await crypto.subtle.importKey(
    "raw",
    encoder.encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"]
  );

  const signatureBuffer = await crypto.subtle.sign(
    "HMAC",
    key,
    encoder.encode(rawBody)
  );
  const expectedSignature =
    "sha256=" +
    Array.from(new Uint8Array(signatureBuffer))
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");

  // Timing-safe comparison
  if (!timingSafeEqual(expectedSignature, signature)) {
    throw new HTTPException(401, { message: "Invalid webhook signature" });
  }

  // Store parsed body for handlers
  c.set("webhookPayload", JSON.parse(rawBody));

  await next();
};

// Timing-safe string comparison to prevent timing attacks
const timingSafeEqual = (a: string, b: string): boolean => {
  // Pad to same length to avoid length-based timing leak
  const maxLen = Math.max(a.length, b.length);
  const paddedA = a.padEnd(maxLen, "\0");
  const paddedB = b.padEnd(maxLen, "\0");

  let result = a.length ^ b.length; // Include length difference in result
  for (let i = 0; i < maxLen; i++) {
    result |= paddedA.charCodeAt(i) ^ paddedB.charCodeAt(i);
  }

  return result === 0;
};
