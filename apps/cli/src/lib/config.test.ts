import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ProjectConfig } from "./config";
import {
  getProjectConfig,
  getProjectConfigPath,
  getProjectConfigSafe,
  isRepoLinked,
  removeProjectConfig,
  saveProjectConfig,
} from "./config";

// Mock node:fs
vi.mock("node:fs", () => ({
  existsSync: vi.fn(),
  mkdirSync: vi.fn(),
  readFileSync: vi.fn(),
  writeFileSync: vi.fn(),
  unlinkSync: vi.fn(),
}));

const createValidProjectConfig = (
  overrides: Partial<ProjectConfig> = {}
): ProjectConfig => ({
  organizationId: "org-123",
  organizationSlug: "test-org",
  ...overrides,
});

describe("project config", () => {
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.clearAllMocks();
    consoleErrorSpy = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
  });

  describe("getProjectConfigPath", () => {
    it("returns correct path for repo root", () => {
      const result = getProjectConfigPath("/repo");

      expect(result).toBe("/repo/.detent/project.json");
    });

    it("handles trailing slash in repo root", () => {
      const result = getProjectConfigPath("/repo/");

      expect(result).toBe("/repo/.detent/project.json");
    });
  });

  describe("getProjectConfigSafe", () => {
    it("returns null config when file does not exist", () => {
      vi.mocked(existsSync).mockReturnValue(false);

      const result = getProjectConfigSafe("/repo");

      expect(result).toEqual({ config: null });
    });

    it("returns config when file exists with valid data", () => {
      const config = createValidProjectConfig();
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue(JSON.stringify(config));

      const result = getProjectConfigSafe("/repo");

      expect(result).toEqual({ config });
    });

    it("returns null config for empty file", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue("");

      const result = getProjectConfigSafe("/repo");

      expect(result).toEqual({ config: null });
    });

    it("returns null config for whitespace-only file", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue("   \n\t  ");

      const result = getProjectConfigSafe("/repo");

      expect(result).toEqual({ config: null });
    });

    it("returns error for invalid JSON", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue("not valid json");

      const result = getProjectConfigSafe("/repo");

      expect(result.config).toBeNull();
      expect(result.error).toContain("invalid JSON");
    });

    it("returns error when missing organizationId", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue(
        JSON.stringify({ organizationSlug: "test-org" })
      );

      const result = getProjectConfigSafe("/repo");

      expect(result.config).toBeNull();
      expect(result.error).toContain(
        "missing organizationId or organizationSlug"
      );
    });

    it("returns error when missing organizationSlug", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue(
        JSON.stringify({ organizationId: "org-123" })
      );

      const result = getProjectConfigSafe("/repo");

      expect(result.config).toBeNull();
      expect(result.error).toContain(
        "missing organizationId or organizationSlug"
      );
    });

    it("returns error for permission denied", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockImplementation(() => {
        const error = new Error("Permission denied") as NodeJS.ErrnoException;
        error.code = "EACCES";
        throw error;
      });

      const result = getProjectConfigSafe("/repo");

      expect(result.config).toBeNull();
      expect(result.error).toContain("permission denied");
    });

    it("returns generic error for unknown errors", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockImplementation(() => {
        throw new Error("Unknown error");
      });

      const result = getProjectConfigSafe("/repo");

      expect(result.config).toBeNull();
      expect(result.error).toContain("failed to load project config");
    });
  });

  describe("getProjectConfig", () => {
    it("returns config when valid", () => {
      const config = createValidProjectConfig();
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue(JSON.stringify(config));

      const result = getProjectConfig("/repo");

      expect(result).toEqual(config);
    });

    it("returns null when file does not exist", () => {
      vi.mocked(existsSync).mockReturnValue(false);

      const result = getProjectConfig("/repo");

      expect(result).toBeNull();
    });

    it("logs warning and returns null for corrupted file", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue("invalid json");

      const result = getProjectConfig("/repo");

      expect(result).toBeNull();
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining("warning:")
      );
    });
  });

  describe("saveProjectConfig", () => {
    it("creates directory if it does not exist", () => {
      vi.mocked(existsSync).mockReturnValue(false);

      saveProjectConfig("/repo", createValidProjectConfig());

      expect(mkdirSync).toHaveBeenCalledWith("/repo/.detent", {
        mode: 0o700,
        recursive: true,
      });
    });

    it("does not create directory if it exists", () => {
      vi.mocked(existsSync).mockReturnValue(true);

      saveProjectConfig("/repo", createValidProjectConfig());

      expect(mkdirSync).not.toHaveBeenCalled();
    });

    it("writes config with correct permissions", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      const config = createValidProjectConfig();

      saveProjectConfig("/repo", config);

      expect(writeFileSync).toHaveBeenCalledWith(
        "/repo/.detent/project.json",
        expect.stringContaining('"organizationId"'),
        { mode: 0o600 }
      );
    });

    it("formats JSON with indentation", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      const config = createValidProjectConfig();

      saveProjectConfig("/repo", config);

      const writtenData = vi.mocked(writeFileSync).mock.calls[0][1] as string;
      expect(writtenData).toContain("\n");
      expect(writtenData.endsWith("\n")).toBe(true);
    });

    it("preserves all config fields", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      const config = createValidProjectConfig({
        organizationId: "custom-org-id",
        organizationSlug: "custom-slug",
      });

      saveProjectConfig("/repo", config);

      const writtenData = vi.mocked(writeFileSync).mock.calls[0][1] as string;
      const parsed = JSON.parse(writtenData) as ProjectConfig;
      expect(parsed.organizationId).toBe("custom-org-id");
      expect(parsed.organizationSlug).toBe("custom-slug");
    });
  });

  describe("removeProjectConfig", () => {
    it("removes config file when it exists", async () => {
      vi.mocked(existsSync).mockReturnValue(true);
      const unlinkSync = vi.fn();
      vi.doMock("node:fs", async () => {
        const actual = await vi.importActual("node:fs");
        return { ...actual, unlinkSync };
      });

      await removeProjectConfig("/repo");

      // Verify it checks for file existence
      expect(existsSync).toHaveBeenCalledWith("/repo/.detent/project.json");
    });

    it("does nothing when config file does not exist", async () => {
      vi.mocked(existsSync).mockReturnValue(false);

      await removeProjectConfig("/repo");

      // Should just complete without error
      expect(existsSync).toHaveBeenCalledWith("/repo/.detent/project.json");
    });
  });

  describe("isRepoLinked", () => {
    it("returns true when project config exists", () => {
      vi.mocked(existsSync).mockReturnValue(true);

      const result = isRepoLinked("/repo");

      expect(result).toBe(true);
      expect(existsSync).toHaveBeenCalledWith("/repo/.detent/project.json");
    });

    it("returns false when project config does not exist", () => {
      vi.mocked(existsSync).mockReturnValue(false);

      const result = isRepoLinked("/repo");

      expect(result).toBe(false);
    });
  });
});
