import { describe, expect, it } from "vitest";
import { toDisplayErrors } from "./format.js";
import type { ParsedError } from "./runner/types.js";

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
