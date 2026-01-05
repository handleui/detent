import { existsSync, unlinkSync } from "node:fs";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { downloadAndVerify } from "./download.js";
import { extractActBinary } from "./extract.js";
import { getActPath, getBinDir, getDetentDir, isInstalled } from "./paths.js";
import { getDownloadUrl } from "./platform.js";
import { verifyActVersion } from "./verify.js";
import { ACT_VERSION } from "./version.js";

export interface InstallOptions {
  readonly onProgress?: (downloaded: number, total: number) => void;
  readonly signal?: AbortSignal;
}

export interface InstallResult {
  readonly installed: boolean;
  readonly path: string;
  readonly version: string;
}

const LOCK_TIMEOUT_MS = 300_000;
const LOCK_CHECK_INTERVAL_MS = 100;

const checkStaleLock = async (lockFile: string): Promise<boolean> => {
  try {
    const lockData = await readFile(join(lockFile, "pid"), "utf-8");
    const { timestamp } = JSON.parse(lockData) as { timestamp: number };
    return Date.now() - timestamp > LOCK_TIMEOUT_MS;
  } catch {
    return true;
  }
};

const acquireLock = async (): Promise<string> => {
  const lockDir = join(getDetentDir(), ".lock");
  const lockFile = join(lockDir, "act-install.lock");
  const startTime = Date.now();

  await mkdir(lockDir, { recursive: true });

  while (true) {
    try {
      await mkdir(lockFile, { mode: 0o755 });
      await writeFile(
        join(lockFile, "pid"),
        JSON.stringify({ pid: process.pid, timestamp: Date.now() }),
        "utf-8"
      );
      return lockFile;
    } catch (error) {
      if (
        error instanceof Error &&
        "code" in error &&
        error.code === "EEXIST"
      ) {
        if (Date.now() - startTime > LOCK_TIMEOUT_MS) {
          const isStale = await checkStaleLock(lockFile);
          if (isStale) {
            await rm(lockFile, { recursive: true, force: true });
            continue;
          }
          throw new Error(
            "Installation lock timeout: another installation may be in progress"
          );
        }
        await new Promise((resolve) =>
          setTimeout(resolve, LOCK_CHECK_INTERVAL_MS)
        );
        continue;
      }
      throw error;
    }
  }
};

const releaseLock = async (lockFile: string): Promise<void> => {
  try {
    await rm(lockFile, { recursive: true, force: true });
  } catch {
    // Ignore errors during lock release
  }
};

export const install = async (
  options: InstallOptions = {}
): Promise<InstallResult> => {
  const lockFile = await acquireLock();

  try {
    if (isInstalled()) {
      return {
        installed: true,
        path: getActPath(),
        version: ACT_VERSION,
      };
    }

    const binaryUrl = getDownloadUrl(ACT_VERSION);
    const checksumUrl = `https://github.com/nektos/act/releases/download/v${ACT_VERSION}/checksums.txt`;

    let platform: string;
    if (process.platform === "win32") {
      platform = "Windows";
    } else if (process.platform === "darwin") {
      platform = "Darwin";
    } else {
      platform = "Linux";
    }

    const arch = process.arch === "x64" ? "x86_64" : "arm64";
    const ext = process.platform === "win32" ? "zip" : "tar.gz";
    const binaryFilename = `act_${platform}_${arch}.${ext}`;

    const binDir = getBinDir();
    const actPath = getActPath();

    await mkdir(binDir, { recursive: true, mode: 0o755 });

    let tempFile: string | undefined;

    try {
      tempFile = await downloadAndVerify(
        binaryUrl,
        checksumUrl,
        binaryFilename,
        options.onProgress
      );

      await extractActBinary(tempFile, actPath);

      if (!existsSync(actPath)) {
        throw new Error("Binary not found after extraction");
      }

      const versionValid = await verifyActVersion(actPath, ACT_VERSION);
      if (!versionValid) {
        unlinkSync(actPath);
        throw new Error(
          `Version verification failed: installed binary does not match expected version ${ACT_VERSION}`
        );
      }

      return {
        installed: true,
        path: actPath,
        version: ACT_VERSION,
      };
    } catch (error) {
      try {
        if (existsSync(actPath)) {
          unlinkSync(actPath);
        }
      } catch {
        // Ignore cleanup errors
      }

      throw new Error(
        `Failed to install act: ${error instanceof Error ? error.message : String(error)}`
      );
    } finally {
      if (tempFile) {
        try {
          unlinkSync(tempFile);
        } catch {
          // Ignore cleanup errors
        }
      }
    }
  } finally {
    await releaseLock(lockFile);
  }
};

export const ensureInstalled = async (
  options: InstallOptions = {}
): Promise<InstallResult> => {
  if (isInstalled()) {
    return {
      installed: true,
      path: getActPath(),
      version: ACT_VERSION,
    };
  }
  return await install(options);
};
