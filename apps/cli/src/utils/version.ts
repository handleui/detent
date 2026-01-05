import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// Injected at compile time via: bun build --define DETENT_VERSION='"x.y.z"'
declare const DETENT_VERSION: string | undefined;

interface PackageJson {
  version?: string;
}

const FALLBACK_VERSION = "0.0.0";

const getVersionFromPackageJson = (): string => {
  try {
    const __dirname = dirname(fileURLToPath(import.meta.url));
    const pkgPath = join(__dirname, "..", "..", "package.json");
    const pkg: PackageJson = JSON.parse(readFileSync(pkgPath, "utf-8"));
    return pkg.version ?? FALLBACK_VERSION;
  } catch {
    return FALLBACK_VERSION;
  }
};

export const getVersion = (): string => {
  // Use compile-time injected version for standalone binaries
  if (typeof DETENT_VERSION !== "undefined") {
    return DETENT_VERSION;
  }
  // Fall back to package.json for development
  return getVersionFromPackageJson();
};
