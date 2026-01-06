#!/usr/bin/env bun

/**
 * Uploads CLI binaries to Vercel Blob.
 * Uses @vercel/blob SDK - no size limits, no API route needed.
 *
 * Required env vars:
 * - BLOB_READ_WRITE_TOKEN: Vercel Blob token (from Vercel dashboard)
 */

import { readdir, readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { del, list, put } from "@vercel/blob";
import { rcompare } from "semver";

const __dirname = dirname(fileURLToPath(import.meta.url));
const CLI_ROOT = join(__dirname, "..");
const DIST_DIR = join(CLI_ROOT, "dist");
const PACKAGE_JSON_PATH = join(CLI_ROOT, "package.json");

const MANIFEST_PATH = "cli/manifest.json";
const MAX_VERSIONS_TO_KEEP = 20;

const log = (msg: string) => console.log(`[upload] ${msg}`);
const fatal = (msg: string): never => {
  console.error(`[upload] ERROR: ${msg}`);
  process.exit(1);
};

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

const withRetry = async <T>(
  fn: () => Promise<T>,
  label: string,
  maxRetries = 3,
  baseDelayMs = 1000
): Promise<T> => {
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      return await fn();
    } catch (err) {
      if (attempt === maxRetries) {
        throw err;
      }
      const delay = baseDelayMs * 2 ** (attempt - 1);
      const message = err instanceof Error ? err.message : String(err);
      log(`${label} failed (attempt ${attempt}/${maxRetries}): ${message}`);
      log(`Retrying in ${delay}ms...`);
      await sleep(delay);
    }
  }
  throw new Error("Unreachable");
};

const getVersion = async (): Promise<string> => {
  const pkg = JSON.parse(await readFile(PACKAGE_JSON_PATH, "utf-8"));
  return pkg.version;
};

interface Archive {
  filename: string;
  path: string;
}

const findArchives = async (): Promise<Archive[]> => {
  const files = await readdir(DIST_DIR);
  return files
    .filter((f) => f.endsWith(".tar.gz") || f.endsWith(".zip"))
    .filter((f) => f.startsWith("dt-"))
    .map((filename) => ({
      filename,
      path: join(DIST_DIR, filename),
    }));
};

const uploadChecksums = async (version: string): Promise<void> => {
  const checksumsPath = join(DIST_DIR, "checksums.txt");
  try {
    const content = await readFile(checksumsPath);
    const blobPath = `cli/v${version}/checksums.txt`;
    await withRetry(
      () =>
        put(blobPath, content, {
          access: "public",
          addRandomSuffix: false,
          allowOverwrite: true,
        }),
      "Upload checksums.txt"
    );
    log("Uploaded checksums.txt");
  } catch {
    log("Warning: checksums.txt not found, skipping");
  }

  // Upload cosign signature files if they exist
  const signatureFiles = ["checksums.txt.sig", "checksums.txt.pem"];
  for (const filename of signatureFiles) {
    const filePath = join(DIST_DIR, filename);
    try {
      const content = await readFile(filePath);
      const blobPath = `cli/v${version}/${filename}`;
      await withRetry(
        () =>
          put(blobPath, content, {
            access: "public",
            addRandomSuffix: false,
            allowOverwrite: true,
          }),
        `Upload ${filename}`
      );
      log(`Uploaded ${filename}`);
    } catch {
      log(`Note: ${filename} not found (cosign signing may not be configured)`);
    }
  }
};

interface Manifest {
  latest: string;
  versions: string[];
  updatedAt: string;
}

const getManifest = async (): Promise<Manifest> => {
  try {
    const { blobs } = await list({ prefix: MANIFEST_PATH });
    if (blobs[0]) {
      const res = await fetch(blobs[0].url);
      return (await res.json()) as Manifest;
    }
  } catch {
    // No manifest yet
  }
  return { latest: "", versions: [], updatedAt: "" };
};

const updateManifest = async (version: string): Promise<Manifest> => {
  const manifest = await getManifest();
  const tag = `v${version}`;

  if (!manifest.versions.includes(tag)) {
    manifest.versions.unshift(tag);
  }

  // Sort descending by semver (handles pre-release versions correctly)
  manifest.versions.sort((a, b) => rcompare(a, b));

  manifest.latest = manifest.versions[0] || tag;
  manifest.updatedAt = new Date().toISOString();

  await withRetry(
    () =>
      put(MANIFEST_PATH, JSON.stringify(manifest, null, 2), {
        access: "public",
        addRandomSuffix: false,
        allowOverwrite: true,
      }),
    "Update manifest"
  );

  log(
    `Updated manifest: latest=${manifest.latest}, total=${manifest.versions.length} versions`
  );
  return manifest;
};

const uploadBinary = async (
  archive: Archive,
  version: string
): Promise<void> => {
  const { filename, path } = archive;
  const blobPath = `cli/v${version}/${filename}`;

  log(`Uploading ${filename}...`);
  const content = await readFile(path);

  const blob = await withRetry(
    () =>
      put(blobPath, content, {
        access: "public",
        addRandomSuffix: false,
        allowOverwrite: true,
      }),
    `Upload ${filename}`
  );

  log(`  → ${blob.url}`);
};

const cleanupOldVersions = async (manifest: Manifest): Promise<void> => {
  if (manifest.versions.length <= MAX_VERSIONS_TO_KEEP) {
    log(`${manifest.versions.length} versions, no cleanup needed`);
    return;
  }

  const toDelete = manifest.versions.slice(MAX_VERSIONS_TO_KEEP);
  log(`Cleaning up ${toDelete.length} old version(s)...`);

  for (const version of toDelete) {
    try {
      const { blobs } = await list({ prefix: `cli/${version}/` });
      if (blobs.length > 0) {
        await del(blobs.map((b) => b.url));
        log(`  Deleted ${version} (${blobs.length} files)`);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      log(`  Warning: failed to delete ${version}: ${message}`);
    }
  }

  // Update manifest without old versions
  manifest.versions = manifest.versions.slice(0, MAX_VERSIONS_TO_KEEP);
  manifest.updatedAt = new Date().toISOString();

  await withRetry(
    () =>
      put(MANIFEST_PATH, JSON.stringify(manifest, null, 2), {
        access: "public",
        addRandomSuffix: false,
        allowOverwrite: true,
      }),
    "Update manifest after cleanup"
  );
};

const main = async () => {
  if (!process.env.BLOB_READ_WRITE_TOKEN) {
    fatal("BLOB_READ_WRITE_TOKEN is required");
  }

  const version = await getVersion();
  log(`Version: ${version}`);

  const archives = await findArchives();
  if (archives.length === 0) {
    fatal(`No archives found in ${DIST_DIR}`);
  }

  log(`Found ${archives.length} archive(s)`);

  // Upload all
  for (const archive of archives) {
    await uploadBinary(archive, version);
  }

  // Upload checksums
  await uploadChecksums(version);

  // Update manifest
  const manifest = await updateManifest(version);

  // Cleanup
  await cleanupOldVersions(manifest);

  log("Done!");
};

main().catch((err) => {
  const message = err instanceof Error ? err.message : String(err);
  fatal(message);
});
