export interface AgentInfo {
  isAgent: boolean;
  name: string;
  version: string;
}

export const detectAgent = (): AgentInfo => {
  if (process.env.CLAUDE_CODE === "1" || process.env.ANTHROPIC_AGENT === "1") {
    return {
      isAgent: true,
      name: "claude-code",
      version: process.env.CLAUDE_CODE_VERSION ?? "unknown",
    };
  }

  if (process.env.CURSOR_TRACE_ID || process.env.CURSOR_SESSION_ID) {
    return { isAgent: true, name: "cursor", version: "unknown" };
  }

  if (process.env.CODEIUM_API_KEY || process.env.WINDSURF_SESSION_ID) {
    return { isAgent: true, name: "windsurf", version: "unknown" };
  }

  if (!process.stdout.isTTY) {
    return { isAgent: true, name: "non-interactive", version: "unknown" };
  }

  return { isAgent: false, name: "", version: "" };
};
