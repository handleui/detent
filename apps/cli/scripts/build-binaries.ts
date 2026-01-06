#!/usr/bin/env bun

/**
 * Builds standalone CLI binaries for all supported platforms using Bun's compile feature.
 * Creates archives and checksums for distribution.
 */

declare const Bun: typeof globalThis.Bun;

import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { mkdir, readFile, rm, stat, writeFile } from "node:fs/promises";
import { basename, dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

interface Target {
  bun: string;
  os: string;
  arch: string;
  ext: string;
  archive: "tar.gz" | "zip";
}

const TARGETS: Target[] = [
  {
    bun: "bun-linux-x64",
    os: "linux",
    arch: "amd64",
    ext: "",
    archive: "tar.gz",
  },
  {
    bun: "bun-linux-arm64",
    os: "linux",
    arch: "arm64",
    ext: "",
    archive: "tar.gz",
  },
  {
    bun: "bun-darwin-x64",
    os: "darwin",
    arch: "amd64",
    ext: "",
    archive: "tar.gz",
  },
  {
    bun: "bun-darwin-arm64",
    os: "darwin",
    arch: "arm64",
    ext: "",
    archive: "tar.gz",
  },
  {
    bun: "bun-windows-x64",
    os: "windows",
    arch: "amd64",
    ext: ".exe",
    archive: "zip",
  },
];

const formatChecksumLine = (checksum: string, filename: string): string =>
  `${checksum}  ${filename}`;

const __dirname = dirname(fileURLToPath(import.meta.url));
const CLI_ROOT = join(__dirname, "..");
const DIST_DIR = join(CLI_ROOT, "dist");
const SRC_ENTRY = join(CLI_ROOT, "src", "index.ts");

const log = (msg: string) => console.log(`[build] ${msg}`);
const fatal = (msg: string): never => {
  console.error(`[build] ERROR: ${msg}`);
  process.exit(1);
};

const getVersion = async (): Promise<string> => {
  const pkgPath = join(CLI_ROOT, "package.json");
  const pkg = JSON.parse(await readFile(pkgPath, "utf-8"));
  return pkg.version;
};

const compileBinary = async (
  target: Target,
  version: string
): Promise<string> => {
  const outPath = join(DIST_DIR, `dt-${target.os}-${target.arch}${target.ext}`);

  log(`Compiling for ${target.os}/${target.arch}...`);

  const proc = Bun.spawn({
    cmd: [
      "bun",
      "build",
      "--compile",
      "--bytecode",
      `--target=${target.bun}`,
      `--define=DETENT_VERSION=${JSON.stringify(version)}`,
      `--define=process.env.NODE_ENV="production"`,
      "--minify",
      SRC_ENTRY,
      `--outfile=${outPath}`,
    ],
    cwd: CLI_ROOT,
    stdout: "pipe",
    stderr: "pipe",
  });

  const exitCode = await proc.exited;
  if (exitCode !== 0) {
    const stderr = await new Response(proc.stderr).text();
    fatal(`Failed to compile for ${target.bun}: ${stderr}`);
  }

  log(`  → ${basename(outPath)}`);
  return outPath;
};

const createTarGz = async (
  binaryPath: string,
  archivePath: string
): Promise<void> => {
  const dir = dirname(binaryPath);
  const { create } = await import("tar");

  await create(
    {
      gzip: true,
      file: archivePath,
      cwd: dir,
    },
    [basename(binaryPath)]
  );
};

const createZip = async (
  binaryPath: string,
  archivePath: string
): Promise<void> => {
  // Use Bun's built-in zip support or shell command
  const proc = Bun.spawn({
    cmd: ["zip", "-j", archivePath, binaryPath],
    stdout: "pipe",
    stderr: "pipe",
  });

  const exitCode = await proc.exited;
  if (exitCode !== 0) {
    const stderr = await new Response(proc.stderr).text();
    fatal(`Failed to create zip: ${stderr}`);
  }
};

const createArchive = async (
  target: Target,
  binaryPath: string
): Promise<string> => {
  const archiveName = `dt-${target.os}-${target.arch}.${target.archive}`;
  const archivePath = join(DIST_DIR, archiveName);

  log(`Creating ${archiveName}...`);

  if (target.archive === "tar.gz") {
    await createTarGz(binaryPath, archivePath);
  } else {
    await createZip(binaryPath, archivePath);
  }

  // Remove the raw binary after archiving
  await rm(binaryPath);

  return archivePath;
};

const calculateChecksum = async (filePath: string): Promise<string> => {
  const hash = createHash("sha256");
  const stream = createReadStream(filePath);

  for await (const chunk of stream) {
    hash.update(chunk);
  }

  return hash.digest("hex");
};

const generateChecksums = async (archivePaths: string[]): Promise<void> => {
  const checksums: string[] = [];

  for (const archivePath of archivePaths) {
    const checksum = await calculateChecksum(archivePath);
    const filename = basename(archivePath);
    checksums.push(formatChecksumLine(checksum, filename));
  }

  const checksumsPath = join(DIST_DIR, "checksums.txt");
  await writeFile(checksumsPath, `${checksums.join("\n")}\n`);
  log("Generated checksums.txt");
};

const main = async () => {
  const version = await getVersion();
  log(`Version: ${version}`);

  // Clean and create dist directory
  await rm(DIST_DIR, { recursive: true, force: true });
  await mkdir(DIST_DIR, { recursive: true });

  // Build all targets in parallel
  const archivePaths = await Promise.all(
    TARGETS.map(async (target) => {
      const binaryPath = await compileBinary(target, version);
      return createArchive(target, binaryPath);
    })
  );

  await generateChecksums(archivePaths);

  log(`\nBuilt ${archivePaths.length} archives:`);
  for (const path of archivePaths) {
    const stats = await stat(path);
    const sizeMB = (stats.size / 1024 / 1024).toFixed(1);
    log(`  ${basename(path)} (${sizeMB} MB)`);
  }

  log("\nDone!");
};

main().catch((err) => fatal(err.message));
