import { spawn } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { compare, valid } from "semver";

const MANIFEST_URL = "https://detent.sh/api/cli/manifest.json";
const INSTALL_SCRIPT_URL = "https://detent.sh/install.sh";
const CACHE_FILE = "update-cache.json";
const CACHE_DURATION_MS = 24 * 60 * 60 * 1000; // 24 hours
const HTTP_TIMEOUT_MS = 5000;
const MAX_RESPONSE_SIZE = 64 * 1024; // 64KB - prevent memory exhaustion
const VERSION_PREFIX_REGEX = /^v/;

interface Manifest {
  latest: string;
  versions: string[];
}

interface UpdateCache {
  lastCheck: number;
  latestVersion: string;
}

interface UpdateCheckResult {
  hasUpdate: boolean;
  latestVersion: string | null;
  currentVersion: string;
}

const getDetentDir = (): string => {
  const home = process.env.DETENT_HOME || homedir();
  return join(home, ".detent");
};

const getCachePath = (): string => join(getDetentDir(), CACHE_FILE);

const loadCache = (): UpdateCache | null => {
  const cachePath = getCachePath();
  if (!existsSync(cachePath)) {
    return null;
  }

  try {
    const data = readFileSync(cachePath, "utf-8");
    return JSON.parse(data) as UpdateCache;
  } catch {
    return null;
  }
};

const saveCache = (cache: UpdateCache): void => {
  const cachePath = getCachePath();
  const dir = dirname(cachePath);

  try {
    if (!existsSync(dir)) {
      mkdirSync(dir, { recursive: true, mode: 0o700 });
    }
    writeFileSync(cachePath, JSON.stringify(cache), { mode: 0o600 });
  } catch {
    // Silent fail - cache is optional
  }
};

const fetchLatestVersion = async (): Promise<string | null> => {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), HTTP_TIMEOUT_MS);

  try {
    const response = await fetch(MANIFEST_URL, { signal: controller.signal });
    clearTimeout(timeoutId);

    if (!response.ok) {
      return null;
    }

    // Limit response size to prevent memory exhaustion
    const contentLength = response.headers.get("content-length");
    if (
      contentLength &&
      Number.parseInt(contentLength, 10) > MAX_RESPONSE_SIZE
    ) {
      return null;
    }

    const text = await response.text();
    if (text.length > MAX_RESPONSE_SIZE) {
      return null;
    }

    const manifest = JSON.parse(text) as Manifest;

    if (!manifest.latest) {
      return null;
    }

    const version = manifest.latest.replace(VERSION_PREFIX_REGEX, "");
    if (!valid(version)) {
      return null;
    }

    return manifest.latest;
  } catch {
    clearTimeout(timeoutId);
    return null;
  }
};

const fetchWithRetry = async (
  maxAttempts = 3,
  delayMs = 500
): Promise<string | null> => {
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    const result = await fetchLatestVersion();
    if (result !== null) {
      return result;
    }

    if (attempt < maxAttempts) {
      await new Promise((resolve) =>
        setTimeout(resolve, delayMs * 2 ** (attempt - 1))
      );
    }
  }
  return null;
};

const compareVersions = (
  current: string,
  latest: string
): { hasUpdate: boolean; latestVersion: string } => {
  const currentClean = current.replace(VERSION_PREFIX_REGEX, "");
  const latestClean = latest.replace(VERSION_PREFIX_REGEX, "");

  if (!(valid(currentClean) && valid(latestClean))) {
    return { hasUpdate: false, latestVersion: latest };
  }

  const result = compare(latestClean, currentClean);
  return {
    hasUpdate: result > 0,
    latestVersion: latest.startsWith("v") ? latest : `v${latest}`,
  };
};

/**
 * Checks if a new version is available.
 * Uses a 24h cache to avoid repeated network calls.
 * Returns silently on errors (cache miss or network failure).
 */
export const checkForUpdate = async (
  currentVersion: string
): Promise<UpdateCheckResult> => {
  const result: UpdateCheckResult = {
    hasUpdate: false,
    latestVersion: null,
    currentVersion,
  };

  // Skip check for dev versions
  if (
    !currentVersion ||
    currentVersion === "dev" ||
    currentVersion === "0.0.0"
  ) {
    return result;
  }

  const cache = loadCache();
  const now = Date.now();

  // Use cache if fresh
  if (cache && now - cache.lastCheck < CACHE_DURATION_MS) {
    const { hasUpdate, latestVersion } = compareVersions(
      currentVersion,
      cache.latestVersion
    );
    return { hasUpdate, latestVersion, currentVersion };
  }

  // Fetch latest version
  const latest = await fetchWithRetry();

  if (latest === null) {
    // Fall back to stale cache if available
    if (cache) {
      const { hasUpdate, latestVersion } = compareVersions(
        currentVersion,
        cache.latestVersion
      );
      return { hasUpdate, latestVersion, currentVersion };
    }
    return result;
  }

  // Update cache
  saveCache({ lastCheck: now, latestVersion: latest });

  const { hasUpdate, latestVersion } = compareVersions(currentVersion, latest);
  return { hasUpdate, latestVersion, currentVersion };
};

/**
 * Runs the update by executing the install script.
 * Streams output to stdout/stderr.
 */
export const runUpdate = (): Promise<boolean> =>
  new Promise((resolve) => {
    const proc = spawn(
      "bash",
      ["-c", `set -o pipefail; curl -fsSL ${INSTALL_SCRIPT_URL} | bash`],
      { stdio: "inherit" }
    );

    proc.on("close", (code) => {
      resolve(code === 0);
    });

    proc.on("error", () => {
      resolve(false);
    });
  });

/**
 * Clears the update cache.
 * Useful for forcing a fresh version check.
 */
export const clearUpdateCache = (): void => {
  const cachePath = getCachePath();
  try {
    rmSync(cachePath, { force: true });
  } catch {
    // Silent fail
  }
};

/**
 * Non-blocking update check.
 * Returns cached result instantly, spawns detached subprocess to refresh if stale.
 */
export const checkForUpdateCached = (
  currentVersion: string
): UpdateCheckResult | null => {
  // Skip check for dev versions
  if (
    !currentVersion ||
    currentVersion === "dev" ||
    currentVersion === "0.0.0"
  ) {
    return null;
  }

  const cache = loadCache();
  const now = Date.now();
  const cacheIsFresh = cache && now - cache.lastCheck < CACHE_DURATION_MS;

  // Spawn background refresh if cache is stale or missing
  if (!cacheIsFresh) {
    spawnBackgroundRefresh();
  }

  // Return cached result if available
  if (cache) {
    const { hasUpdate, latestVersion } = compareVersions(
      currentVersion,
      cache.latestVersion
    );
    return { hasUpdate, latestVersion, currentVersion };
  }

  return null;
};

/**
 * Spawns a detached subprocess to refresh the update cache.
 * The subprocess runs independently and won't block process exit.
 */
const spawnBackgroundRefresh = (): void => {
  const manifestUrl = MANIFEST_URL;
  const cachePath = getCachePath();
  const script = `
    const https = require('https');
    const fs = require('fs');
    const path = require('path');

    const MANIFEST_URL = '${manifestUrl}';
    const CACHE_PATH = '${cachePath}';

    https.get(MANIFEST_URL, { timeout: 5000 }, (res) => {
      if (res.statusCode !== 200) return;
      let data = '';
      res.on('data', (chunk) => { data += chunk; });
      res.on('end', () => {
        try {
          const manifest = JSON.parse(data);
          if (manifest.latest) {
            const dir = path.dirname(CACHE_PATH);
            if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
            fs.writeFileSync(CACHE_PATH, JSON.stringify({
              lastCheck: Date.now(),
              latestVersion: manifest.latest
            }), { mode: 0o600 });
          }
        } catch {}
      });
    }).on('error', () => {});
  `;

  try {
    const child = spawn(process.execPath, ["-e", script], {
      detached: true,
      stdio: "ignore",
    });
    child.unref();
  } catch {
    // Silent fail - background refresh is optional
  }
};
