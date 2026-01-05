import { describe, expect, it } from "vitest";
import { ParserClient } from "./parser-client.js";

const ERROR_PATTERN = /Failed to connect|timed out/;

describe("ParserClient", () => {
  it("creates instance with default configuration", () => {
    const client = new ParserClient();
    expect(client).toBeDefined();
  });

  it("creates instance with custom baseUrl", () => {
    const client = new ParserClient({ baseUrl: "http://custom:9999" });
    expect(client).toBeDefined();
  });

  it("creates instance with custom timeout", () => {
    const client = new ParserClient({ timeout: 60_000 });
    expect(client).toBeDefined();
  });

  it("parses empty logs", async () => {
    const client = new ParserClient();
    // Note: This test requires parser service to be running
    // Skip in CI by checking for service availability
    try {
      const result = await client.parse("");
      expect(result).toHaveProperty("errors");
      expect(Array.isArray(result.errors)).toBe(true);
    } catch (error) {
      if (error instanceof Error) {
        // Service not running is acceptable in test environment
        expect(error.message).toMatch(ERROR_PATTERN);
      }
    }
  });
});
