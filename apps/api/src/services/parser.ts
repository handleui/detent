import type {
  ErrorCategory,
  ErrorSource,
  ExtractedError,
} from "@detent/parser";

export type { ExtractedError } from "@detent/parser";

interface ParseOptions {
  format: "github-actions" | "act";
  logs: string;
}

interface ParseResult {
  errors: ExtractedError[];
  summary: {
    total: number;
    byCategory: Record<ErrorCategory, number>;
    bySource: Record<ErrorSource, number>;
  };
}

export const parseService = {
  // Parse CI logs and extract errors
  parse: async (_options: ParseOptions): Promise<ParseResult> => {
    // Stub implementation - parsing will be wired up when the API is ready.
    // The actual parsing uses @detent/parser:
    // - parseGitHubLogs(options.logs) for GitHub Actions format
    // - parseActLogs(options.logs) for Act format

    // Placeholder await for stub (will be used when implementing)
    await Promise.resolve();

    return {
      errors: [],
      summary: {
        total: 0,
        byCategory: {} as Record<ErrorCategory, number>,
        bySource: {} as Record<ErrorSource, number>,
      },
    };
  },
};
