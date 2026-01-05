import { describe, expect, it } from "bun:test";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { getVersion } from "./version.ts";

const __dirname = dirname(fileURLToPath(import.meta.url));
const SEMVER_PATTERN = /^\d+\.\d+\.\d+/;

describe("getVersion", () => {
  it("returns valid semver format", () => {
    const version = getVersion();
    expect(version).toMatch(SEMVER_PATTERN);
  });

  it("returns package.json version in development", () => {
    const version = getVersion();
    const pkgPath = join(__dirname, "..", "..", "package.json");
    const pkg = JSON.parse(readFileSync(pkgPath, "utf-8"));

    // In dev mode (no DETENT_VERSION defined), should match package.json
    expect(version).toBe(pkg.version);
  });
});

describe("version module source", () => {
  it("has FALLBACK_VERSION constant set to 0.0.0", () => {
    const source = readFileSync(join(__dirname, "version.ts"), "utf-8");
    expect(source).toContain('FALLBACK_VERSION = "0.0.0"');
  });

  it("declares DETENT_VERSION for compile-time injection", () => {
    const source = readFileSync(join(__dirname, "version.ts"), "utf-8");
    expect(source).toContain("declare const DETENT_VERSION");
  });

  it("checks typeof DETENT_VERSION before using it", () => {
    const source = readFileSync(join(__dirname, "version.ts"), "utf-8");
    expect(source).toContain('typeof DETENT_VERSION !== "undefined"');
  });
});
