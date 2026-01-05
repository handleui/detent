import { describe, expect, test } from "vitest";
import { getActPath, getBinDir, getDetentDir } from "./paths.js";
import { detectPlatform, getDownloadUrl } from "./platform.js";
import { ACT_VERSION } from "./version.js";

describe("version", () => {
  test("ACT_VERSION is a valid semver string", () => {
    expect(ACT_VERSION).toMatch(/^\d+\.\d+\.\d+$/);
    expect(ACT_VERSION).toBe("0.2.83");
  });
});

describe("platform", () => {
  test("detectPlatform returns valid platform info", () => {
    const platform = detectPlatform();
    expect(platform).toHaveProperty("os");
    expect(platform).toHaveProperty("arch");
    expect(platform.os).toMatch(/^(Darwin|Linux|Windows)$/);
    expect(platform.arch).toMatch(/^(x86_64|arm64)$/);
  });

  test("getDownloadUrl constructs correct URL with version", () => {
    const url = getDownloadUrl("0.2.83");
    expect(url).toContain(
      "https://github.com/nektos/act/releases/download/v0.2.83/act_"
    );
    expect(url).toMatch(/\.(tar\.gz|zip)$/);
  });

  test("getDownloadUrl includes platform and architecture", () => {
    const url = getDownloadUrl("0.2.83");
    expect(url).toMatch(/act_(Darwin|Linux|Windows)_(x86_64|arm64)/);
  });

  test("getDownloadUrl uses tar.gz for Unix platforms", () => {
    const url = getDownloadUrl("0.2.83");
    if (process.platform === "darwin" || process.platform === "linux") {
      expect(url).toMatch(/\.tar\.gz$/);
    }
  });

  test("getDownloadUrl uses zip for Windows", () => {
    if (process.platform === "win32") {
      const url = getDownloadUrl("0.2.83");
      expect(url).toMatch(/\.zip$/);
    } else {
      expect(true).toBe(true);
    }
  });
});

describe("paths", () => {
  test("getDetentDir respects DETENT_HOME env var", () => {
    const originalEnv = process.env.DETENT_HOME;

    process.env.DETENT_HOME = "/custom/home";
    expect(getDetentDir()).toBe("/custom/home/.detent");

    if (originalEnv) {
      process.env.DETENT_HOME = originalEnv;
    } else {
      process.env.DETENT_HOME = undefined;
    }
  });

  test("getDetentDir defaults to user home directory", () => {
    const originalEnv = process.env.DETENT_HOME;
    process.env.DETENT_HOME = undefined;

    const dir = getDetentDir();
    expect(dir).toContain(".detent");
    expect(dir).not.toBe("/.detent");

    if (originalEnv) {
      process.env.DETENT_HOME = originalEnv;
    }
  });

  test("getBinDir returns .detent/bin path", () => {
    const binDir = getBinDir();
    expect(binDir).toContain(".detent");
    expect(binDir).toContain("bin");
  });

  test("getActPath includes version in filename", () => {
    const actPath = getActPath();
    expect(actPath).toContain("act-");
    expect(actPath).toContain(ACT_VERSION);
  });

  test("getActPath matches platform-specific extension", () => {
    const actPath = getActPath();
    if (process.platform === "win32") {
      expect(actPath).toMatch(/act-.*\.exe$/);
    } else {
      expect(actPath).not.toMatch(/\.exe$/);
      expect(actPath).toMatch(/act-[\d.]+$/);
    }
  });

  test("getActPath is in bin directory", () => {
    const actPath = getActPath();
    const binDir = getBinDir();
    expect(actPath).toContain(binDir);
  });
});
