import type { ExtractedError } from "@detent/parser";

export type { ExtractedError } from "@detent/parser";

interface HealOptions {
  errors: ExtractedError[];
  repoUrl: string;
  branch: string;
  // Future: Add Claude API key from org context
  // Future: Add budget limits
}

interface HealEvent {
  type: "status" | "tool_call" | "message" | "patch" | "complete" | "error";
  data: unknown;
}

export const healerService = {
  // Run healing loop with streaming events
  async *heal(_options: HealOptions): AsyncGenerator<HealEvent> {
    // Stub implementation - healing will be wired up when the API is ready.
    // The actual healing will:
    // 1. Clone repo to workspace
    // 2. Initialize HealLoop with tools from @detent/healing
    // 3. Stream Claude responses
    // 4. Apply patches and verify fixes

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
