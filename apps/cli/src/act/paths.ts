import { existsSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import { ACT_VERSION } from "./version.js";

/**
 * Gets the global detent directory (~/.detent)
 * Used for shared resources like the act binary
 */
export const getGlobalDetentDir = (): string => {
  const home = process.env.DETENT_HOME || homedir();
  return join(home, ".detent");
};

/**
 * @deprecated Use getGlobalDetentDir() instead
 */
export const getDetentDir = getGlobalDetentDir;

export const getBinDir = (): string => {
  return join(getGlobalDetentDir(), "bin");
};

export const getActPath = (): string => {
  const binDir = getBinDir();
  const binaryName = `act-${ACT_VERSION}${process.platform === "win32" ? ".exe" : ""}`;
  return join(binDir, binaryName);
};

export const isInstalled = (): boolean => {
  const actPath = getActPath();
  return existsSync(actPath);
};
