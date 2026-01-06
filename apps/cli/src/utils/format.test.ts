import { describe, expect, it } from "vitest";
import { formatDuration, formatDurationMs } from "./format.js";

describe("formatDuration", () => {
  it("formats seconds under 60", () => {
    expect(formatDuration(0)).toBe("0s");
    expect(formatDuration(45)).toBe("45s");
    expect(formatDuration(59)).toBe("59s");
  });

  it("formats minutes and seconds", () => {
    expect(formatDuration(60)).toBe("1m 0s");
    expect(formatDuration(90)).toBe("1m 30s");
    expect(formatDuration(125)).toBe("2m 5s");
    expect(formatDuration(3661)).toBe("61m 1s");
  });
});

describe("formatDurationMs", () => {
  it("formats milliseconds under 60 seconds with decimal", () => {
    expect(formatDurationMs(0)).toBe("0.0s");
    expect(formatDurationMs(1500)).toBe("1.5s");
    expect(formatDurationMs(59_999)).toBe("60.0s");
  });

  it("formats minutes and seconds without decimal", () => {
    expect(formatDurationMs(60_000)).toBe("1m 0s");
    expect(formatDurationMs(90_000)).toBe("1m 30s");
  });
});
