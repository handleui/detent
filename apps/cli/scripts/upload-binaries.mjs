#!/usr/bin/env node

/**
 * Uploads CLI binaries directly to Vercel Blob.
 * Uses @vercel/blob SDK - no size limits, no API route needed.
 *
 * Required env vars:
 * - BLOB_READ_WRITE_TOKEN: Vercel Blob token (from Vercel dashboard)
 */

import { readdir, readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { del, list, put } from "@vercel/blob";

const __dirname = dirname(fileURLToPath(import.meta.url));
const CLI_ROOT = join(__dirname, "..");
const DIST_DIR = join(CLI_ROOT, "dist");
const PACKAGE_JSON_PATH = join(CLI_ROOT, "package.json");

const MANIFEST_PATH = "releases/manifest.json";
const MAX_VERSIONS_TO_KEEP = 20;

const log = (msg) => console.log(`[upload] ${msg}`);
const fatal = (msg) => {
  console.error(`[upload] ERROR: ${msg}`);
  process.exit(1);
};

const getVersion = async () => {
  const pkg = JSON.parse(await readFile(PACKAGE_JSON_PATH, "utf-8"));
  return pkg.version;
};

const findArchives = async () => {
  const files = await readdir(DIST_DIR);
  return files
    .filter((f) => f.endsWith(".tar.gz") || f.endsWith(".zip"))
    .filter((f) => f.startsWith("dt-"))
    .map((filename) => ({
      filename,
      path: join(DIST_DIR, filename),
    }));
};

const uploadChecksums = async (version) => {
  const checksumsPath = join(DIST_DIR, "checksums.txt");
  try {
    const content = await readFile(checksumsPath);
    const blobPath = `releases/v${version}/checksums.txt`;
    await put(blobPath, content, {
      access: "public",
      addRandomSuffix: false,
      allowOverwrite: true,
    });
    log("Uploaded checksums.txt");
  } catch {
    log("Warning: checksums.txt not found, skipping");
  }
};

const getManifest = async () => {
  try {
    const { blobs } = await list({ prefix: MANIFEST_PATH });
    if (blobs[0]) {
      const res = await fetch(blobs[0].url);
      return await res.json();
    }
  } catch {
    // No manifest yet
  }
  return { latest: "", versions: [], updatedAt: "" };
};

const updateManifest = async (version) => {
  const manifest = await getManifest();
  const tag = `v${version}`;

  if (!manifest.versions.includes(tag)) {
    manifest.versions.unshift(tag);
  }

  // Sort descending by semver
  manifest.versions.sort((a, b) => {
    const [aMaj, aMin, aPat] = a.slice(1).split(".").map(Number);
    const [bMaj, bMin, bPat] = b.slice(1).split(".").map(Number);
    return bMaj - aMaj || bMin - aMin || bPat - aPat;
  });

  manifest.latest = manifest.versions[0] || tag;
  manifest.updatedAt = new Date().toISOString();

  await put(MANIFEST_PATH, JSON.stringify(manifest, null, 2), {
    access: "public",
    addRandomSuffix: false,
    allowOverwrite: true,
  });

  log(
    `Updated manifest: latest=${manifest.latest}, total=${manifest.versions.length} versions`
  );
  return manifest;
};

const uploadBinary = async (archive, version) => {
  const { filename, path } = archive;
  const blobPath = `releases/v${version}/${filename}`;

  log(`Uploading ${filename}...`);
  const content = await readFile(path);

  const blob = await put(blobPath, content, {
    access: "public",
    addRandomSuffix: false,
    allowOverwrite: true,
  });

  log(`  → ${blob.url}`);
  return blob;
};

const cleanupOldVersions = async (manifest) => {
  if (manifest.versions.length <= MAX_VERSIONS_TO_KEEP) {
    log(`${manifest.versions.length} versions, no cleanup needed`);
    return;
  }

  const toDelete = manifest.versions.slice(MAX_VERSIONS_TO_KEEP);
  log(`Cleaning up ${toDelete.length} old version(s)...`);

  for (const version of toDelete) {
    try {
      const { blobs } = await list({ prefix: `releases/${version}/` });
      if (blobs.length > 0) {
        await del(blobs.map((b) => b.url));
        log(`  Deleted ${version} (${blobs.length} files)`);
      }
    } catch (err) {
      log(`  Warning: failed to delete ${version}: ${err.message}`);
    }
  }

  // Update manifest without old versions
  manifest.versions = manifest.versions.slice(0, MAX_VERSIONS_TO_KEEP);
  manifest.updatedAt = new Date().toISOString();

  await put(MANIFEST_PATH, JSON.stringify(manifest, null, 2), {
    access: "public",
    addRandomSuffix: false,
    allowOverwrite: true,
  });
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

main().catch((err) => fatal(err.message));
