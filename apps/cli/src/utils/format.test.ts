import { describe, expect, it } from "vitest";
import type { ParsedError } from "../runner/types.js";
import { formatDuration, formatDurationMs, toDisplayErrors } from "./format.js";

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

describe("toDisplayErrors", () => {
  it("transforms empty array", () => {
    expect(toDisplayErrors([])).toEqual([]);
  });

  it("transforms error with all fields", () => {
    const parsedErrors: ParsedError[] = [
      {
        errorId: "123",
        contentHash: "abc",
        message: "Type error",
        filePath: "src/index.ts",
        line: 10,
        column: 5,
        severity: "error",
        ruleId: "TS2322",
        category: "type-check",
        source: "typescript",
      },
    ];

    const result = toDisplayErrors(parsedErrors);

    expect(result).toHaveLength(1);
    expect(result[0]).toEqual({
      message: "Type error",
      file: "src/index.ts",
      line: 10,
      column: 5,
      severity: "error",
      ruleId: "TS2322",
      category: "type-check",
    });
  });

  it("transforms warning severity", () => {
    const parsedErrors: ParsedError[] = [
      {
        errorId: "456",
        contentHash: "def",
        message: "Unused variable",
        severity: "warning",
      },
    ];

    const result = toDisplayErrors(parsedErrors);

    expect(result[0]?.severity).toBe("warning");
  });

  it("normalizes non-warning severity to error", () => {
    const parsedErrors: ParsedError[] = [
      {
        errorId: "789",
        contentHash: "ghi",
        message: "Critical failure",
        severity: "critical",
      },
    ];

    const result = toDisplayErrors(parsedErrors);

    expect(result[0]?.severity).toBe("error");
  });

  it("handles multiple errors", () => {
    const parsedErrors: ParsedError[] = [
      {
        errorId: "1",
        contentHash: "a",
        message: "Error 1",
        severity: "error",
      },
      {
        errorId: "2",
        contentHash: "b",
        message: "Error 2",
        severity: "warning",
      },
    ];

    const result = toDisplayErrors(parsedErrors);

    expect(result).toHaveLength(2);
    expect(result[0]?.message).toBe("Error 1");
    expect(result[1]?.message).toBe("Error 2");
  });

  it("handles errors with undefined optional fields", () => {
    const parsedErrors: ParsedError[] = [
      {
        errorId: "minimal",
        contentHash: "hash",
        message: "Basic error",
        severity: "error",
      },
    ];

    const result = toDisplayErrors(parsedErrors);

    expect(result[0]).toEqual({
      message: "Basic error",
      file: undefined,
      line: undefined,
      column: undefined,
      severity: "error",
      ruleId: undefined,
      category: undefined,
    });
  });
});
