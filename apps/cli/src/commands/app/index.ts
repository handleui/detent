import { defineCommand, runCommand } from "citty";

export const appCommand = defineCommand({
  meta: {
    name: "app",
    description: "Manage GitHub App installation",
  },
  subCommands: {
    install: () => import("./install.js").then((m) => m.installCommand),
    status: () => import("./status.js").then((m) => m.statusCommand),
  },
  run: async () => {
    // Default action: show status
    const { statusCommand } = await import("./status.js");
    await runCommand(statusCommand, { rawArgs: [] });
  },
});
