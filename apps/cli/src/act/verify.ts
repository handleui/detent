import { execFile } from "node:child_process";
import { promisify } from "node:util";
import type { ACT_VERSION } from "./version.js";

const execFileAsync = promisify(execFile);

const ACT_VERSION_REGEX = /act version ([\d.]+)/i;

export const verifyActVersion = async (
  actPath: string,
  expectedVersion: typeof ACT_VERSION
): Promise<boolean> => {
  try {
    const { stdout } = await execFileAsync(actPath, ["--version"], {
      timeout: 5000,
    });

    const versionMatch = stdout.match(ACT_VERSION_REGEX);
    if (!versionMatch) {
      return false;
    }

    const installedVersion = versionMatch[1];
    return installedVersion === expectedVersion;
  } catch {
    return false;
  }
};
