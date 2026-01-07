import { defineCommand, runCommand } from "citty";

export const linkCommand = defineCommand({
  meta: {
    name: "link",
    description: "Link your GitHub account to your Detent team",
  },
  subCommands: {
    status: () => import("./status.js").then((m) => m.statusCommand),
    unlink: () => import("./unlink.js").then((m) => m.unlinkCommand),
  },
  run: async () => {
    // Default action: run the GitHub linking flow
    const { githubLinkCommand } = await import("./github.js");
    await runCommand(githubLinkCommand, { rawArgs: [] });
  },
});
