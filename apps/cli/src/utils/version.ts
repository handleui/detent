import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

interface PackageJson {
  version?: string;
}

const FALLBACK_VERSION = "0.0.0";

export const getVersion = (): string => {
  try {
    const __dirname = dirname(fileURLToPath(import.meta.url));
    const pkgPath = join(__dirname, "..", "..", "package.json");
    const pkg: PackageJson = JSON.parse(readFileSync(pkgPath, "utf-8"));
    return pkg.version ?? FALLBACK_VERSION;
  } catch {
    return FALLBACK_VERSION;
  }
};
