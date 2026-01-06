import { createHash } from "node:crypto";
import { createReadStream, createWriteStream, unlinkSync } from "node:fs";
import { mkdir } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Readable } from "node:stream";
import { pipeline } from "node:stream/promises";

export type ProgressCallback = (downloaded: number, total: number) => void;

const MAX_SIZE = 100 * 1024 * 1024;
const TIMEOUT_MS = 60_000;
const MAX_RETRIES = 3;
const INITIAL_RETRY_DELAY_MS = 1000;

const WHITESPACE_REGEX = /\s+/;

const validateUrl = (url: string): void => {
  try {
    const parsed = new URL(url);
    if (parsed.protocol !== "https:") {
      throw new Error("URL must use HTTPS protocol");
    }
    if (
      parsed.hostname !== "github.com" &&
      !parsed.hostname.endsWith(".githubusercontent.com")
    ) {
      throw new Error("URL must be from github.com or githubusercontent.com");
    }
  } catch (error) {
    if (error instanceof TypeError) {
      throw new Error(`Invalid URL: ${url}`);
    }
    throw error;
  }
};

const sleep = (ms: number): Promise<void> => {
  return new Promise((resolve) => setTimeout(resolve, ms));
};

export const computeSha256 = (filePath: string): Promise<string> => {
  const hash = createHash("sha256");
  const stream = createReadStream(filePath);

  return new Promise((resolve, reject) => {
    stream.on("data", (chunk) => hash.update(chunk));
    stream.on("end", () => resolve(hash.digest("hex")));
    stream.on("error", reject);
  });
};

const downloadFileOnce = async (
  url: string,
  onProgress?: ProgressCallback
): Promise<string> => {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);

  try {
    const response = await fetch(url, { signal: controller.signal });

    if (!response.ok) {
      throw new Error(`Download failed: HTTP ${response.status}`);
    }

    if (!response.body) {
      throw new Error("Download failed: no response body");
    }

    const total = Number.parseInt(
      response.headers.get("content-length") || "0",
      10
    );

    if (total > MAX_SIZE) {
      throw new Error(`Download exceeds maximum size of ${MAX_SIZE} bytes`);
    }

    const tempDir = tmpdir();
    await mkdir(tempDir, { recursive: true });
    const tempFile = join(
      tempDir,
      `act-download-${Date.now()}-${Math.random().toString(36).slice(2)}.tmp`
    );

    const writeStream = createWriteStream(tempFile);
    let downloaded = 0;

    const body = response.body;
    const transformStream = new Readable({
      async read() {
        const reader = body.getReader();
        try {
          while (true) {
            const { done, value } = await reader.read();
            if (done) {
              this.push(null);
              break;
            }

            downloaded += value.length;

            if (downloaded > MAX_SIZE) {
              reader.cancel();
              throw new Error(
                `Download exceeds maximum size of ${MAX_SIZE} bytes`
              );
            }

            if (onProgress) {
              onProgress(downloaded, total);
            }

            this.push(value);
          }
        } catch (error) {
          reader.cancel();
          throw error;
        }
      },
    });

    try {
      await pipeline(transformStream, writeStream);
      clearTimeout(timeoutId);
      return tempFile;
    } catch (error) {
      try {
        unlinkSync(tempFile);
      } catch {
        // Ignore cleanup errors
      }
      throw error;
    }
  } catch (error) {
    clearTimeout(timeoutId);
    if (error instanceof Error && error.name === "AbortError") {
      throw new Error(`Download timed out after ${TIMEOUT_MS / 1000} seconds`);
    }
    throw error;
  }
};

export const downloadFile = async (
  url: string,
  onProgress?: ProgressCallback
): Promise<string> => {
  validateUrl(url);

  let lastError: Error | undefined;

  for (let attempt = 0; attempt < MAX_RETRIES; attempt++) {
    try {
      return await downloadFileOnce(url, onProgress);
    } catch (error) {
      lastError = error instanceof Error ? error : new Error(String(error));

      if (
        error instanceof Error &&
        error.message.includes("exceeds maximum size")
      ) {
        throw error;
      }

      if (attempt < MAX_RETRIES - 1) {
        const delay = INITIAL_RETRY_DELAY_MS * 2 ** attempt;
        await sleep(delay);
      }
    }
  }

  throw new Error(
    `Download failed after ${MAX_RETRIES} attempts: ${lastError?.message || "Unknown error"}`
  );
};

export const downloadAndVerify = async (
  binaryUrl: string,
  checksumUrl: string,
  binaryFilename: string,
  onProgress?: ProgressCallback
): Promise<string> => {
  const binaryFile = await downloadFile(binaryUrl, onProgress);
  let checksumFile: string | undefined;

  try {
    checksumFile = await downloadFile(checksumUrl);

    const { readFile } = await import("node:fs/promises");
    const checksumContent = await readFile(checksumFile, "utf-8");

    const lines = checksumContent.split("\n");
    let expectedHash: string | undefined;

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) {
        continue;
      }

      const parts = trimmed.split(WHITESPACE_REGEX);
      if (parts.length >= 2 && parts[1] === binaryFilename && parts[0]) {
        expectedHash = parts[0].toLowerCase();
        break;
      }
    }

    if (!expectedHash) {
      throw new Error(
        `Checksum not found for ${binaryFilename} in checksums.txt`
      );
    }

    const actualHash = await computeSha256(binaryFile);

    if (actualHash !== expectedHash) {
      unlinkSync(binaryFile);
      throw new Error(
        `Checksum verification failed. Expected ${expectedHash}, got ${actualHash}`
      );
    }

    return binaryFile;
  } finally {
    if (checksumFile) {
      try {
        unlinkSync(checksumFile);
      } catch {
        // Ignore cleanup errors
      }
    }
  }
};
