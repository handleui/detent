// biome-ignore-all lint/performance/noBarrelFile: This is the act module's public API
export type { ProgressCallback } from "./download.js";
export { computeSha256, downloadAndVerify, downloadFile } from "./download.js";
export { extractActBinary } from "./extract.js";
export type { InstallOptions, InstallResult } from "./install.js";
export { ensureInstalled, install } from "./install.js";
export { getActPath, getBinDir, getDetentDir, isInstalled } from "./paths.js";
export type { PlatformInfo } from "./platform.js";
export { detectPlatform, getDownloadUrl } from "./platform.js";
export { verifyActVersion } from "./verify.js";
export { ACT_VERSION } from "./version.js";
