import { WorkOS } from "@workos-inc/node";

/**
 * WorkOS client instance
 * clientId is required for sealed session methods (loadSealedSession, refreshAndSealSessionData)
 */
export const workos = new WorkOS(process.env.WORKOS_API_KEY, {
  clientId: process.env.WORKOS_CLIENT_ID,
});
