import { describe, expect, it } from "vitest";
import { ActOutputParser } from "./act-output-parser.js";

describe("ActOutputParser", () => {
  describe("parseLine", () => {
    it("returns empty array for non-detent lines", () => {
      const parser = new ActOutputParser();

      expect(parser.parseLine("")).toEqual([]);
      expect(parser.parseLine("Regular log output")).toEqual([]);
      expect(parser.parseLine("[Build] Step 1/10")).toEqual([]);
    });

    it("parses manifest marker", () => {
      const parser = new ActOutputParser();
      const manifest = {
        v: 2,
        jobs: [
          {
            id: "build",
            name: "Build",
            sensitive: false,
            steps: ["Checkout", "Install", "Build"],
          },
        ],
      };
      const base64 = Buffer.from(JSON.stringify(manifest)).toString("base64");
      const line = `::detent::manifest::v2::b64::${base64}`;

      const events = parser.parseLine(line);

      expect(events).toHaveLength(1);
      expect(events[0]?.type).toBe("manifest");
      expect(parser.hasManifest()).toBe(true);

      const manifestEvent = events[0] as { type: "manifest"; jobs: unknown[] };
      expect(manifestEvent.jobs).toHaveLength(1);
      expect(manifestEvent.jobs[0]).toMatchObject({
        id: "build",
        name: "Build",
        sensitive: false,
      });
    });

    it("parses job-start marker", () => {
      const parser = new ActOutputParser();
      const line = "::detent::job-start::build_job";

      const events = parser.parseLine(line);

      expect(events).toHaveLength(1);
      expect(events[0]).toEqual({
        type: "job",
        jobId: "build_job",
        action: "start",
      });
    });

    it("parses job-end success marker", () => {
      const parser = new ActOutputParser();
      const line = "::detent::job-end::build_job::success";

      const events = parser.parseLine(line);

      expect(events).toHaveLength(1);
      expect(events[0]).toEqual({
        type: "job",
        jobId: "build_job",
        action: "finish",
        success: true,
      });
    });

    it("parses job-end failure marker", () => {
      const parser = new ActOutputParser();
      const line = "::detent::job-end::test_job::failure";

      const events = parser.parseLine(line);

      expect(events).toHaveLength(1);
      expect(events[0]).toEqual({
        type: "job",
        jobId: "test_job",
        action: "finish",
        success: false,
      });
    });

    it("parses step-start marker", () => {
      const parser = new ActOutputParser();
      const line = "::detent::step-start::build_job::2::Run tests";

      const events = parser.parseLine(line);

      expect(events).toHaveLength(1);
      expect(events[0]).toEqual({
        type: "step",
        jobId: "build_job",
        stepIdx: 2,
        stepName: "Run tests",
      });
    });

    it("strips ANSI codes before parsing", () => {
      const parser = new ActOutputParser();
      const line = "\x1b[32m::detent::job-start::build\x1b[0m";

      const events = parser.parseLine(line);

      expect(events).toHaveLength(1);
      expect(events[0]).toEqual({
        type: "job",
        jobId: "build",
        action: "start",
      });
    });

    it("handles hyphenated job IDs", () => {
      const parser = new ActOutputParser();
      const line = "::detent::job-start::my-build-job";

      const events = parser.parseLine(line);

      expect(events).toHaveLength(1);
      expect(events[0]).toMatchObject({
        jobId: "my-build-job",
      });
    });

    it("returns empty for invalid manifest payload", () => {
      const parser = new ActOutputParser();
      const line = "::detent::manifest::v2::b64::not-valid-base64!!!";

      const events = parser.parseLine(line);

      expect(events).toEqual([]);
      expect(parser.hasManifest()).toBe(false);
    });

    it("returns empty for manifest with wrong version", () => {
      const parser = new ActOutputParser();
      const invalidManifest = { v: 1, jobs: [] };
      const base64 = Buffer.from(JSON.stringify(invalidManifest)).toString(
        "base64"
      );
      const line = `::detent::manifest::v2::b64::${base64}`;

      const events = parser.parseLine(line);

      expect(events).toEqual([]);
    });

    it("returns empty for step with invalid index", () => {
      const parser = new ActOutputParser();
      const line = "::detent::step-start::job::abc::Step name";

      const events = parser.parseLine(line);

      expect(events).toEqual([]);
    });

    it("returns empty for step with negative index", () => {
      const parser = new ActOutputParser();
      const line = "::detent::step-start::job::-1::Step name";

      const events = parser.parseLine(line);

      expect(events).toEqual([]);
    });
  });

  describe("getManifest", () => {
    it("returns undefined before manifest received", () => {
      const parser = new ActOutputParser();
      expect(parser.getManifest()).toBeUndefined();
    });

    it("returns manifest after parsing", () => {
      const parser = new ActOutputParser();
      const manifest = {
        v: 2,
        jobs: [{ id: "test", name: "Test", sensitive: false, steps: [] }],
      };
      const base64 = Buffer.from(JSON.stringify(manifest)).toString("base64");
      parser.parseLine(`::detent::manifest::v2::b64::${base64}`);

      const result = parser.getManifest();

      expect(result).toBeDefined();
      expect(result?.v).toBe(2);
      expect(result?.jobs).toHaveLength(1);
    });
  });
});
