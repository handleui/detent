/**
 * WorkOS AuthKit JWT verification
 *
 * Verifies access tokens issued by WorkOS AuthKit using JWKS.
 * Tokens are validated for issuer, audience, and signature.
 */

import type { JWTPayload } from "jose";
import { createRemoteJWKSet, jwtVerify } from "jose";

export interface WorkOSJWTPayload extends JWTPayload {
  sub: string;
  sid?: string;
  org_id?: string;
  role?: string;
  permissions?: string[];
}

interface VerifyConfig {
  clientId: string;
  subdomain: string;
}

const jwksCache = new Map<string, ReturnType<typeof createRemoteJWKSet>>();

const getJWKS = (subdomain: string) => {
  const cached = jwksCache.get(subdomain);
  if (cached) {
    return cached;
  }

  const jwks = createRemoteJWKSet(
    new URL(`https://${subdomain}.authkit.app/oauth2/jwks`)
  );
  jwksCache.set(subdomain, jwks);
  return jwks;
};

export const verifyAccessToken = async (
  token: string,
  config: VerifyConfig
): Promise<WorkOSJWTPayload> => {
  const jwks = getJWKS(config.subdomain);

  const { payload } = await jwtVerify(token, jwks, {
    issuer: `https://${config.subdomain}.authkit.app`,
    audience: config.clientId,
  });

  return payload as WorkOSJWTPayload;
};
