import {
  chmodSync,
  createReadStream,
  createWriteStream,
  unlinkSync,
} from "node:fs";
import { mkdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, dirname, join, normalize, resolve, sep } from "node:path";
import { pipeline } from "node:stream/promises";
import { createGunzip } from "node:zlib";
import { extract as tarExtract } from "tar";
import unzipper from "unzipper";

const MAX_EXTRACTED_SIZE = 200 * 1024 * 1024;

const validatePath = (extractPath: string, targetDir: string): void => {
  const normalizedExtractPath = normalize(resolve(targetDir, extractPath));
  const normalizedTargetDir = normalize(resolve(targetDir));

  if (!normalizedExtractPath.startsWith(normalizedTargetDir + sep)) {
    throw new Error(
      `Path traversal detected: ${extractPath} resolves outside target directory`
    );
  }
};

export const extractActBinary = async (
  archivePath: string,
  targetPath: string
): Promise<void> => {
  const isWindows = process.platform === "win32";
  const isZip = archivePath.endsWith(".zip");

  if (isWindows || isZip) {
    await extractFromZip(archivePath, targetPath);
  } else {
    await extractFromTarGz(archivePath, targetPath);
  }
};

const extractFromTarGz = async (
  archivePath: string,
  targetPath: string
): Promise<void> => {
  const tempDir = join(
    tmpdir(),
    `act-extract-${Date.now()}-${Math.random().toString(36).slice(2)}`
  );
  await mkdir(tempDir, { recursive: true });

  try {
    const fileStream = createReadStream(archivePath);
    const gunzip = createGunzip();

    await pipeline(
      fileStream,
      gunzip,
      tarExtract({
        cwd: tempDir,
        strict: true,
        filter: (path: string) => {
          validatePath(path, tempDir);
          const name = basename(path);
          return name === "act" || name === "act.exe";
        },
      })
    );

    const binaryName = process.platform === "win32" ? "act.exe" : "act";
    const extractedBinary = join(tempDir, binaryName);

    const targetDir = dirname(targetPath);
    await mkdir(targetDir, { recursive: true });

    const tempTarget = `${targetPath}.tmp`;
    const readStream = createReadStream(extractedBinary);
    const writeStream = createWriteStream(tempTarget);

    let extracted = 0;
    readStream.on("data", (chunk: Buffer) => {
      extracted += chunk.length;
      if (extracted > MAX_EXTRACTED_SIZE) {
        readStream.destroy();
        writeStream.destroy();
        throw new Error(
          `Extracted file exceeds maximum size of ${MAX_EXTRACTED_SIZE} bytes`
        );
      }
    });

    await pipeline(readStream, writeStream);

    try {
      unlinkSync(targetPath);
    } catch {
      // Ignore if target doesn't exist
    }

    const fs = await import("node:fs/promises");
    await fs.rename(tempTarget, targetPath);

    if (process.platform !== "win32") {
      chmodSync(targetPath, 0o755);
    }
  } catch (error) {
    if (error instanceof Error && error.message.includes("TAR_ENTRY_INVALID")) {
      throw new Error("act binary not found in archive");
    }
    throw error;
  } finally {
    await rm(tempDir, { recursive: true, force: true });
  }
};

const extractFromZip = async (
  archivePath: string,
  targetPath: string
): Promise<void> => {
  const tempDir = join(
    tmpdir(),
    `act-extract-${Date.now()}-${Math.random().toString(36).slice(2)}`
  );
  await mkdir(tempDir, { recursive: true });

  try {
    let binaryFound = false;

    await pipeline(
      createReadStream(archivePath),
      unzipper.Parse(),
      // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: Async generator handles ZIP streaming in single cohesive unit; splitting would harm readability
      async function* (source: AsyncIterable<unzipper.Entry>) {
        for await (const entry of source) {
          validatePath(entry.path, tempDir);
          const name = basename(entry.path);

          if (
            name.toLowerCase() === "act.exe" ||
            name.toLowerCase() === "act"
          ) {
            binaryFound = true;
            const extractedPath = join(tempDir, name);
            const writeStream = createWriteStream(extractedPath);

            let extracted = 0;
            for await (const chunk of entry) {
              extracted += chunk.length;
              if (extracted > MAX_EXTRACTED_SIZE) {
                throw new Error(
                  `Extracted file exceeds maximum size of ${MAX_EXTRACTED_SIZE} bytes`
                );
              }
              writeStream.write(chunk);
            }

            writeStream.end();
            await new Promise((resolve, reject) => {
              writeStream.on("finish", resolve);
              writeStream.on("error", reject);
            });

            const targetDir = dirname(targetPath);
            await mkdir(targetDir, { recursive: true });

            try {
              unlinkSync(targetPath);
            } catch {
              // Ignore if target doesn't exist
            }

            const fs = await import("node:fs/promises");
            await fs.rename(extractedPath, targetPath);

            if (process.platform !== "win32") {
              chmodSync(targetPath, 0o755);
            }

            yield;
            return;
          }
          entry.autodrain();
        }
      }
    );

    if (!binaryFound) {
      throw new Error("act binary not found in archive");
    }
  } finally {
    await rm(tempDir, { recursive: true, force: true });
  }
};
