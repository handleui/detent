import { join } from "node:path";
import type { GlobalConfig } from "@detent/persistence";
import { render } from "ink-testing-library";
import { fs, vol } from "memfs";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("node:fs", () => fs);
vi.mock("node:fs/promises", () => fs.promises);

vi.mock("../../utils/version.js", () => ({
  getVersion: () => "0.0.1",
}));

const setupMockFS = (config: GlobalConfig = {}) => {
  const detentDir = "/test-detent";
  const configPath = join(detentDir, "detent.json");

  vol.fromJSON({
    [configPath]: JSON.stringify(config, null, 2),
  });

  process.env.DETENT_HOME = detentDir;
  delete process.env.ANTHROPIC_API_KEY;
};

const cleanupMockFS = () => {
  vol.reset();
  delete process.env.DETENT_HOME;
};

describe("ConfigEditor", () => {
  let ConfigEditor: () => JSX.Element;

  beforeEach(async () => {
    const module = await import("./edit.js");
    ConfigEditor = module.ConfigEditor;
  });

  afterEach(() => {
    cleanupMockFS();
  });

  describe("Config loading", () => {
    it("should load and display complete config from file system", async () => {
      setupMockFS({
        apiKey: "sk-ant-api03-test-key-1234567890",
        model: "claude-sonnet-4-5",
        budgetPerRunUsd: 5,
        budgetMonthlyUsd: 100,
        timeoutMins: 15,
      });

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("API Key");
      expect(output).toContain("****7890");
      expect(output).toContain("claude-sonnet-4-5");
      expect(output).toContain("$5.00");
      expect(output).toContain("$100.00");
      expect(output).toContain("15 min");
    });

    it("should load partial config and show defaults for missing values", async () => {
      setupMockFS({
        apiKey: "sk-ant-api03-partial-config-key",
        model: "claude-opus-4-5",
      });

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("****-key");
      expect(output).toContain("claude-opus-4-5");
      expect(output).toContain("unlimited");
      expect(output).toContain("none");
    });

    it("should handle empty config file", async () => {
      setupMockFS({});

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("not set");
      expect(output).toContain("default");
      expect(output).toContain("unlimited");
      expect(output).toContain("none");
    });

    it("should handle non-existent config file", async () => {
      vol.fromJSON({});
      process.env.DETENT_HOME = "/test-detent";

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("not set");
      expect(output).toContain("default");
    });
  });

  describe("UI rendering", () => {
    it("should render header with version and command name", async () => {
      setupMockFS({});

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("Detent v0.0.1");
      expect(output).toContain("config");
    });

    it("should render all config field labels", async () => {
      setupMockFS({});

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("API Key");
      expect(output).toContain("Model");
      expect(output).toContain("Budget/Run");
      expect(output).toContain("Budget/Month");
      expect(output).toContain("Timeout/Run");
    });

    it("should show focus indicator on first field", async () => {
      setupMockFS({});

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("›");
    });

    it("should show context-appropriate help text", async () => {
      setupMockFS({});

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("type or paste");
    });

    it("should show keyboard shortcuts in footer", async () => {
      setupMockFS({});

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("↑↓ navigate");
      expect(output).toContain("c clear");
      expect(output).toContain("q/esc close");
    });
  });

  describe("API key masking", () => {
    it("should mask API key showing only last 4 characters", async () => {
      setupMockFS({
        apiKey: "sk-ant-api03-secret-key-abcdef1234",
      });

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("****1234");
      expect(output).not.toContain("secret");
      expect(output).not.toContain("abcdef");
    });

    it("should show 'not set' for missing API key", async () => {
      setupMockFS({});

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      const lines = output.split("\n");
      const apiKeyLine = lines.find((line) => line.includes("API Key"));

      expect(apiKeyLine).toContain("not set");
    });
  });

  describe("Value formatting", () => {
    it("should format budget values as USD currency", async () => {
      setupMockFS({
        budgetPerRunUsd: 12.5,
        budgetMonthlyUsd: 250,
      });

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("$12.50");
      expect(output).toContain("$250.00");
    });

    it("should show 'unlimited' for zero budget values", async () => {
      setupMockFS({
        budgetPerRunUsd: 0,
        budgetMonthlyUsd: 0,
      });

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      const lines = output.split("\n");
      const budgetRunLine = lines.find((line) => line.includes("Budget/Run"));
      const budgetMonthLine = lines.find((line) =>
        line.includes("Budget/Month")
      );

      expect(budgetRunLine).toContain("unlimited");
      expect(budgetMonthLine).toContain("unlimited");
    });

    it("should format timeout with minutes label", async () => {
      setupMockFS({
        timeoutMins: 30,
      });

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("30 min");
    });

    it("should show 'none' for zero timeout", async () => {
      setupMockFS({
        timeoutMins: 0,
      });

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      const lines = output.split("\n");
      const timeoutLine = lines.find((line) => line.includes("Timeout/Run"));

      expect(timeoutLine).toContain("none");
    });

    it("should show 'default' for empty model", async () => {
      setupMockFS({});

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      const lines = output.split("\n");
      const modelLine = lines.find((line) => line.includes("Model"));

      expect(modelLine).toContain("default");
    });
  });

  describe("Edge cases", () => {
    it("should handle all fields at maximum values", async () => {
      setupMockFS({
        apiKey: "sk-ant-api03-max-config-test",
        model: "claude-opus-4-5",
        budgetPerRunUsd: 100,
        budgetMonthlyUsd: 1000,
        timeoutMins: 60,
      });

      const { lastFrame } = render(<ConfigEditor />);
      const output = lastFrame() ?? "";

      expect(output).toContain("****test");
      expect(output).toContain("claude-opus-4-5");
      expect(output).toContain("$100.00");
      expect(output).toContain("$1000.00");
      expect(output).toContain("60 min");
    });

    it("should handle all model options correctly", async () => {
      const models = [
        "claude-opus-4-5",
        "claude-sonnet-4-5",
        "claude-haiku-4-5",
      ];

      for (const model of models) {
        setupMockFS({ model });

        const { lastFrame } = render(<ConfigEditor />);
        const output = lastFrame() ?? "";

        expect(output).toContain(model);

        cleanupMockFS();
      }
    });
  });
});
