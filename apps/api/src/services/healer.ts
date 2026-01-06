// TODO: Uncomment when wiring to @detent/healing
// import { HealLoop, type HealConfig } from "@detent/healing";

// Stub types until we wire to @detent/healing
interface ExtractedError {
  errorId: string;
  message: string;
  filePath?: string;
  line?: number;
}

interface HealOptions {
  errors: ExtractedError[];
  repoUrl: string;
  branch: string;
  // TODO: Add Claude API key from org context
  // TODO: Add budget limits
}

interface HealEvent {
  type: "status" | "tool_call" | "message" | "patch" | "complete" | "error";
  data: unknown;
}

export const healerService = {
  // Run healing loop with streaming events
  async *heal(_options: HealOptions): AsyncGenerator<HealEvent> {
    // TODO: Implement actual healing loop
    // TODO: Clone repo to workspace
    // TODO: Initialize HealLoop with tools
    // TODO: Stream Claude responses
    // TODO: Apply patches and verify fixes

    // Placeholder await for stub (will be used when implementing)
    await Promise.resolve();

    yield {
      type: "status",
      data: { phase: "initializing" },
    };

    yield {
      type: "status",
      data: { phase: "stub", message: "Healing not yet implemented" },
    };

    yield {
      type: "complete",
      data: { success: false, reason: "stub" },
    };
  },
};
