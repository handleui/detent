/**
 * WorkOS User Management JWT verification
 *
 * Verifies access tokens issued by WorkOS User Management (CLI Auth) using JWKS.
 * Tokens are validated for issuer and signature.
 *
 * User Management tokens differ from AuthKit tokens:
 * - JWKS: https://api.workos.com/sso/jwks/{clientId}
 * - Issuer: https://api.workos.com/user_management/{clientId}
 * - No audience claim (aud is undefined)
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
}

const jwksCache = new Map<string, ReturnType<typeof createRemoteJWKSet>>();

const getJWKS = (clientId: string) => {
  const cached = jwksCache.get(clientId);
  if (cached) {
    return cached;
  }

  // User Management tokens use this JWKS endpoint
  const jwks = createRemoteJWKSet(
    new URL(`https://api.workos.com/sso/jwks/${clientId}`)
  );
  jwksCache.set(clientId, jwks);
  return jwks;
};

export const verifyAccessToken = async (
  token: string,
  config: VerifyConfig
): Promise<WorkOSJWTPayload> => {
  const jwks = getJWKS(config.clientId);

  // User Management tokens have issuer format: https://api.workos.com/user_management/{clientId}
  // They don't have an audience claim
  const { payload } = await jwtVerify(token, jwks, {
    issuer: `https://api.workos.com/user_management/${config.clientId}`,
  });

  return payload as WorkOSJWTPayload;
};
