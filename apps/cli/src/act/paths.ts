import { existsSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import { ACT_VERSION } from "./version.js";

export const getDetentDir = (): string => {
  const home = process.env.DETENT_HOME || homedir();
  return join(home, ".detent");
};

export const getBinDir = (): string => {
  return join(getDetentDir(), "bin");
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
