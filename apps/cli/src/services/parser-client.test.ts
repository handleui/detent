import { describe, expect, it } from "vitest";
import { ParserClient } from "./parser-client.js";

describe("ParserClient", () => {
  it("creates instance", () => {
    const client = new ParserClient();
    expect(client).toBeDefined();
  });

  it("throws on empty string", () => {
    const client = new ParserClient();
    expect(() => client.parse("")).toThrow("Logs must be a non-empty string");
  });

  it("throws on non-string input", () => {
    const client = new ParserClient();
    expect(() => client.parse(null as unknown as string)).toThrow(
      "Logs must be a non-empty string"
    );
  });

  it("parses TypeScript errors with act prefix", () => {
    const client = new ParserClient();
    const logs = `[Build/build] src/index.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.`;

    const result = client.parse(logs);

    expect(result.errors.length).toBeGreaterThan(0);
    const tsError = result.errors.find((e) => e.source === "typescript");
    expect(tsError).toBeDefined();
    expect(tsError?.filePath).toBe("src/index.ts");
    expect(tsError?.line).toBe(10);
    expect(tsError?.message).toContain("Type 'string' is not assignable");
  });

  it("parses Go compiler errors", () => {
    const client = new ParserClient();
    const logs = "[Test/test] main.go:15:2: undefined: foo";

    const result = client.parse(logs);

    expect(result.errors.length).toBeGreaterThan(0);
    const goError = result.errors.find((e) => e.source === "go");
    expect(goError).toBeDefined();
    expect(goError?.filePath).toBe("main.go");
    expect(goError?.line).toBe(15);
    expect(goError?.column).toBe(2);
  });

  it("returns empty errors for clean logs", () => {
    const client = new ParserClient();
    const logs = `[Build/build] Build completed successfully!
[Build/build] All tests passed.`;

    const result = client.parse(logs);

    expect(result.errors).toEqual([]);
  });

  it("extracts multiple errors from mixed output", () => {
    const client = new ParserClient();
    const logs = `[Build/build] src/a.ts(1,1): error TS2304: Cannot find name 'unknown'.
[Build/build] src/b.ts(2,1): error TS2304: Cannot find name 'missing'.`;

    const result = client.parse(logs);

    expect(result.errors.length).toBe(2);
  });

  it("generates unique error IDs", () => {
    const client = new ParserClient();
    const logs = `[Build/build] src/a.ts(1,1): error TS2304: Cannot find name 'x'.
[Build/build] src/b.ts(1,1): error TS2304: Cannot find name 'y'.`;

    const result = client.parse(logs);

    const ids = result.errors.map((e) => e.errorId);
    const uniqueIds = new Set(ids);
    expect(uniqueIds.size).toBe(ids.length);
  });

  it("includes content hash for deduplication", () => {
    const client = new ParserClient();
    const logs =
      "[Build/build] src/index.ts(10,5): error TS2322: Type error message.";

    const result = client.parse(logs);

    expect(result.errors.length).toBe(1);
    expect(result.errors[0]?.contentHash).toBeDefined();
    expect(typeof result.errors[0]?.contentHash).toBe("string");
    expect(result.errors[0]?.contentHash?.length).toBeGreaterThan(0);
  });

  it("parses errors without act prefix", () => {
    const client = new ParserClient();
    const logs = "main.go:10:5: undefined: someFunc";

    const result = client.parse(logs);

    expect(result.errors.length).toBe(1);
    expect(result.errors[0]?.filePath).toBe("main.go");
  });

  it("deduplicates identical errors", () => {
    const client = new ParserClient();
    const logs = `[Build/build] main.go:10:5: undefined: foo
[Build/build] main.go:10:5: undefined: foo
[Build/build] main.go:10:5: undefined: foo`;

    const result = client.parse(logs);

    expect(result.errors.length).toBe(1);
  });

  it("preserves errors at different locations", () => {
    const client = new ParserClient();
    const logs = `[Build/build] main.go:10:5: undefined: x
[Build/build] main.go:20:5: undefined: x
[Build/build] utils.go:10:5: undefined: x`;

    const result = client.parse(logs);

    expect(result.errors.length).toBe(3);
  });
});
