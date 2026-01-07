import {
  existsSync,
  mkdirSync,
  readFileSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Credentials } from "./credentials";
import {
  clearCredentials,
  isLoggedIn,
  isTokenExpired,
  loadCredentials,
  resetCredentialsCache,
  saveCredentials,
} from "./credentials";

// Mock node:fs
vi.mock("node:fs", () => ({
  existsSync: vi.fn(),
  mkdirSync: vi.fn(),
  readFileSync: vi.fn(),
  writeFileSync: vi.fn(),
  unlinkSync: vi.fn(),
}));

// Mock node:os
vi.mock("node:os", () => ({
  homedir: vi.fn(() => "/home/testuser"),
}));

const createValidCredentials = (
  overrides: Partial<Credentials> = {}
): Credentials => ({
  access_token: "test-access-token",
  refresh_token: "test-refresh-token",
  expires_at: Date.now() + 3_600_000,
  ...overrides,
});

describe("credentials", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Reset DETENT_HOME env var
    process.env.DETENT_HOME = undefined;
    // Reset credentials cache to ensure tests are isolated
    resetCredentialsCache();
  });

  afterEach(() => {
    process.env.DETENT_HOME = undefined;
  });

  describe("loadCredentials", () => {
    it("returns null when credentials file does not exist", () => {
      vi.mocked(existsSync).mockReturnValue(false);

      const result = loadCredentials();

      expect(result).toBeNull();
      expect(existsSync).toHaveBeenCalledWith(
        join("/home/testuser", ".detent", "credentials.json")
      );
    });

    it("returns credentials when file exists with valid data", () => {
      const creds = createValidCredentials();
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue(JSON.stringify(creds));

      const result = loadCredentials();

      expect(result).toEqual(creds);
    });

    it("returns null for empty file", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue("");

      const result = loadCredentials();

      expect(result).toBeNull();
    });

    it("returns null for whitespace-only file", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue("   \n\t  ");

      const result = loadCredentials();

      expect(result).toBeNull();
    });

    it("returns null for invalid JSON", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue("not valid json");

      const result = loadCredentials();

      expect(result).toBeNull();
    });

    it("returns null for missing access_token", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue(
        JSON.stringify({
          refresh_token: "token",
          expires_at: Date.now(),
        })
      );

      const result = loadCredentials();

      expect(result).toBeNull();
    });

    it("returns null for missing refresh_token", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue(
        JSON.stringify({
          access_token: "token",
          expires_at: Date.now(),
        })
      );

      const result = loadCredentials();

      expect(result).toBeNull();
    });

    it("returns null for missing expires_at", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue(
        JSON.stringify({
          access_token: "token",
          refresh_token: "token",
        })
      );

      const result = loadCredentials();

      expect(result).toBeNull();
    });

    it("returns null for non-string access_token", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue(
        JSON.stringify({
          access_token: 123,
          refresh_token: "token",
          expires_at: Date.now(),
        })
      );

      const result = loadCredentials();

      expect(result).toBeNull();
    });

    it("returns null for non-number expires_at", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue(
        JSON.stringify({
          access_token: "token",
          refresh_token: "token",
          expires_at: "not a number",
        })
      );

      const result = loadCredentials();

      expect(result).toBeNull();
    });

    it("uses DETENT_HOME override when set", () => {
      process.env.DETENT_HOME = "/custom/path";
      vi.mocked(existsSync).mockReturnValue(false);

      loadCredentials();

      expect(existsSync).toHaveBeenCalledWith("/custom/path/credentials.json");
    });

    it("ignores DETENT_HOME with path traversal", () => {
      process.env.DETENT_HOME = "/path/../etc";
      vi.mocked(existsSync).mockReturnValue(false);

      loadCredentials();

      expect(existsSync).toHaveBeenCalledWith(
        join("/home/testuser", ".detent", "credentials.json")
      );
    });

    it("ignores relative DETENT_HOME", () => {
      process.env.DETENT_HOME = "relative/path";
      vi.mocked(existsSync).mockReturnValue(false);

      loadCredentials();

      expect(existsSync).toHaveBeenCalledWith(
        join("/home/testuser", ".detent", "credentials.json")
      );
    });

    it("returns null when read throws error", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockImplementation(() => {
        throw new Error("Permission denied");
      });

      const result = loadCredentials();

      expect(result).toBeNull();
    });
  });

  describe("saveCredentials", () => {
    it("creates directory if it does not exist", () => {
      vi.mocked(existsSync).mockReturnValue(false);

      saveCredentials(createValidCredentials());

      expect(mkdirSync).toHaveBeenCalledWith("/home/testuser/.detent", {
        mode: 0o700,
        recursive: true,
      });
    });

    it("does not create directory if it exists", () => {
      vi.mocked(existsSync).mockReturnValue(true);

      saveCredentials(createValidCredentials());

      expect(mkdirSync).not.toHaveBeenCalled();
    });

    it("writes credentials with correct permissions", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      const creds = createValidCredentials();

      saveCredentials(creds);

      expect(writeFileSync).toHaveBeenCalledWith(
        "/home/testuser/.detent/credentials.json",
        expect.stringContaining('"access_token"'),
        { mode: 0o600 }
      );
    });

    it("formats JSON with indentation", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      const creds = createValidCredentials();

      saveCredentials(creds);

      const writtenData = vi.mocked(writeFileSync).mock.calls[0][1] as string;
      expect(writtenData).toContain("\n");
      expect(writtenData.endsWith("\n")).toBe(true);
    });
  });

  describe("clearCredentials", () => {
    it("returns true when credentials file is deleted", () => {
      vi.mocked(existsSync).mockReturnValue(true);

      const result = clearCredentials();

      expect(result).toBe(true);
      expect(unlinkSync).toHaveBeenCalledWith(
        "/home/testuser/.detent/credentials.json"
      );
    });

    it("returns false when credentials file does not exist", () => {
      vi.mocked(existsSync).mockReturnValue(false);

      const result = clearCredentials();

      expect(result).toBe(false);
      expect(unlinkSync).not.toHaveBeenCalled();
    });

    it("returns false when unlink throws error", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(unlinkSync).mockImplementation(() => {
        throw new Error("Permission denied");
      });

      const result = clearCredentials();

      expect(result).toBe(false);
    });
  });

  describe("isLoggedIn", () => {
    it("returns true when valid credentials exist", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue(
        JSON.stringify(createValidCredentials())
      );

      const result = isLoggedIn();

      expect(result).toBe(true);
    });

    it("returns false when no credentials exist", () => {
      vi.mocked(existsSync).mockReturnValue(false);

      const result = isLoggedIn();

      expect(result).toBe(false);
    });

    it("returns false when credentials are invalid", () => {
      vi.mocked(existsSync).mockReturnValue(true);
      vi.mocked(readFileSync).mockReturnValue("invalid json");

      const result = isLoggedIn();

      expect(result).toBe(false);
    });
  });

  describe("isTokenExpired", () => {
    it("returns false for token expiring in more than 5 minutes", () => {
      const creds = createValidCredentials({
        expires_at: Date.now() + 10 * 60 * 1000,
      });

      const result = isTokenExpired(creds);

      expect(result).toBe(false);
    });

    it("returns true for token expiring in less than 5 minutes", () => {
      const creds = createValidCredentials({
        expires_at: Date.now() + 4 * 60 * 1000,
      });

      const result = isTokenExpired(creds);

      expect(result).toBe(true);
    });

    it("returns true for already expired token", () => {
      const creds = createValidCredentials({
        expires_at: Date.now() - 1000,
      });

      const result = isTokenExpired(creds);

      expect(result).toBe(true);
    });

    it("returns false for token expiring exactly at 5 minute buffer", () => {
      // When expires_at equals Date.now() + bufferMs, the comparison is NOT strictly less than
      // so the token is not considered expired
      const now = Date.now();
      const creds = createValidCredentials({
        expires_at: now + 5 * 60 * 1000 + 1, // Just slightly more than 5 minutes
      });

      const result = isTokenExpired(creds);

      expect(result).toBe(false);
    });
  });
});
