// TODO: Uncomment when wiring to @detent/parser
// import { parseGitHubLogs, parseActLogs, type ExtractedError } from "@detent/parser";

// Stub types until we wire to @detent/parser
interface ExtractedError {
  errorId: string;
  message: string;
  filePath?: string;
  line?: number;
  column?: number;
  category: string;
  source: string;
  severity: "error" | "warning";
}

interface ParseOptions {
  format: "github-actions" | "act";
  logs: string;
}

interface ParseResult {
  errors: ExtractedError[];
  summary: {
    total: number;
    byCategory: Record<string, number>;
    bySource: Record<string, number>;
  };
}

export const parseService = {
  // Parse CI logs and extract errors
  parse: async (_options: ParseOptions): Promise<ParseResult> => {
    // TODO: Implement actual parsing
    // if (options.format === "github-actions") {
    //   return parseGitHubLogs(options.logs);
    // }
    // return parseActLogs(options.logs);

    // Placeholder await for stub (will be used when implementing)
    await Promise.resolve();

    return {
      errors: [],
      summary: {
        total: 0,
        byCategory: {},
        bySource: {},
      },
    };
  },
};
