import { describe, expect, test } from "vitest";
import {
  ErrEmptyRequired,
  ErrFieldTooLong,
  ErrIDTooLong,
  ErrInvalidConfidence,
  ErrInvalidID,
  ErrInvalidPath,
  ErrInvalidStatus,
  ErrPathTraversal,
  validateAssignmentStatus,
  validateConfidence,
  validateFixStatus,
  validateID,
  validatePath,
} from "./validation.js";

describe("validateID", () => {
  test("accepts valid alphanumeric IDs", () => {
    expect(() => validateID("abc123")).not.toThrow();
    expect(() => validateID("test-id_123")).not.toThrow();
  });

  test("throws ErrEmptyRequired for empty string", () => {
    expect(() => validateID("")).toThrow(ErrEmptyRequired);
  });

  test("throws ErrIDTooLong for 129+ chars", () => {
    expect(() => validateID("a".repeat(129))).toThrow(ErrIDTooLong);
  });

  test("throws ErrInvalidID for invalid characters", () => {
    expect(() => validateID("has spaces")).toThrow(ErrInvalidID);
    expect(() => validateID("../etc/passwd")).toThrow(ErrInvalidID);
  });

  test("throws ErrInvalidID when starting with hyphen or underscore", () => {
    expect(() => validateID("-startsWithHyphen")).toThrow(ErrInvalidID);
    expect(() => validateID("_startsWithUnderscore")).toThrow(ErrInvalidID);
  });
});

describe("validatePath", () => {
  test("allows empty path", () => {
    expect(() => validatePath("")).not.toThrow();
  });

  test("throws ErrPathTraversal for path traversal", () => {
    expect(() => validatePath("../etc/passwd")).toThrow(ErrPathTraversal);
  });

  test("throws ErrInvalidPath for null byte", () => {
    expect(() => validatePath("file\0.txt")).toThrow(ErrInvalidPath);
  });

  test("throws ErrFieldTooLong for too long path", () => {
    expect(() => validatePath("a".repeat(4097))).toThrow(ErrFieldTooLong);
  });
});

describe("validateConfidence", () => {
  test("accepts valid range 0-100", () => {
    expect(() => validateConfidence(0)).not.toThrow();
    expect(() => validateConfidence(50)).not.toThrow();
    expect(() => validateConfidence(100)).not.toThrow();
  });

  test("throws ErrInvalidConfidence for out of range", () => {
    expect(() => validateConfidence(-1)).toThrow(ErrInvalidConfidence);
    expect(() => validateConfidence(101)).toThrow(ErrInvalidConfidence);
  });
});

describe("validateAssignmentStatus", () => {
  test("accepts all valid statuses", () => {
    for (const status of [
      "assigned",
      "in_progress",
      "completed",
      "failed",
      "expired",
    ] as const) {
      expect(() => validateAssignmentStatus(status)).not.toThrow();
    }
  });

  test("throws ErrInvalidStatus for invalid status", () => {
    expect(() => validateAssignmentStatus("invalid" as never)).toThrow(
      ErrInvalidStatus
    );
  });
});

describe("validateFixStatus", () => {
  test("accepts all valid statuses", () => {
    for (const status of [
      "pending",
      "applied",
      "rejected",
      "superseded",
    ] as const) {
      expect(() => validateFixStatus(status)).not.toThrow();
    }
  });

  test("throws ErrInvalidStatus for invalid status", () => {
    expect(() => validateFixStatus("invalid" as never)).toThrow(
      ErrInvalidStatus
    );
  });
});
