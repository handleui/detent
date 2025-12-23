#!/usr/bin/env node

const { existsSync, mkdirSync, chmodSync } = require("node:fs");
const { join } = require("node:path");
const { pipeline } = require("node:stream");
const { promisify } = require("node:util");
const https = require("node:https");
const { createGunzip } = require("node:zlib");
const tar = require("tar-fs");

const streamPipeline = promisify(pipeline);

const PACKAGE_JSON = require("./package.json");

const getPlatform = () => {
  const platform = process.platform;
  if (platform === "win32") {
    return "windows";
  }
  if (platform === "darwin") {
    return "darwin";
  }
  if (platform === "linux") {
    return "linux";
  }
  throw new Error(`Unsupported platform: ${platform}`);
};

const getArch = () => {
  const arch = process.arch;
  if (arch === "x64") {
    return "amd64";
  }
  if (arch === "arm64") {
    return "arm64";
  }
  throw new Error(`Unsupported architecture: ${arch}`);
};

const getExtension = () => (process.platform === "win32" ? "zip" : "tar.gz");

const getBinaryName = () => (process.platform === "win32" ? "dt.exe" : "dt");

const download = (url) =>
  new Promise((resolve, reject) => {
    https
      .get(url, (response) => {
        if (response.statusCode === 302 || response.statusCode === 301) {
          return download(response.headers.location).then(resolve, reject);
        }
        if (response.statusCode !== 200) {
          reject(new Error(`Failed to download: ${response.statusCode}`));
          return;
        }
        resolve(response);
      })
      .on("error", reject);
  });

const install = async () => {
  try {
    const platform = getPlatform();
    const arch = getArch();
    const ext = getExtension();
    const version = PACKAGE_JSON.version;

    const url = PACKAGE_JSON.goBinary.url
      .replace("{{version}}", version)
      .replace("{{platform}}", platform)
      .replace("{{arch}}", arch)
      .replace("{{ext}}", ext);

    const binDir = join(__dirname, PACKAGE_JSON.goBinary.path);
    const binaryName = getBinaryName();
    const binaryPath = join(binDir, binaryName);

    if (!existsSync(binDir)) {
      mkdirSync(binDir, { recursive: true });
    }

    console.log(`Downloading binary from ${url}...`);

    const response = await download(url);

    if (process.platform === "win32") {
      const AdmZip = require("adm-zip");
      const chunks = [];

      await new Promise((resolve, reject) => {
        response.on("data", (chunk) => chunks.push(chunk));
        response.on("end", resolve);
        response.on("error", reject);
      });

      const buffer = Buffer.concat(chunks);
      const zip = new AdmZip(buffer);
      zip.extractAllTo(binDir, true);
    } else {
      await streamPipeline(response, createGunzip(), tar.extract(binDir));
    }

    if (process.platform !== "win32") {
      chmodSync(binaryPath, 0o755);
    }

    console.log(`Binary installed successfully at ${binaryPath}`);
  } catch (error) {
    console.error("Failed to install binary:", error.message);
    console.error("\nYou can manually download the binary from:");
    console.error(
      `https://github.com/handleui/detent/releases/tag/v${PACKAGE_JSON.version}`
    );

    // Exit gracefully in CI or during development (version 0.0.0)
    if (process.env.CI || PACKAGE_JSON.version === "0.0.0") {
      console.log("Skipping binary installation in CI/development mode");
      process.exit(0);
    }

    process.exit(1);
  }
};

install();
