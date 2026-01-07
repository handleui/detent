import { defineCommand, runCommand } from "citty";

export const orgCommand = defineCommand({
  meta: {
    name: "org",
    description: "Manage organizations",
  },
  subCommands: {
    create: () => import("./create.js").then((m) => m.createCommand),
    list: () => import("./list.js").then((m) => m.listCommand),
    status: () => import("./status.js").then((m) => m.statusCommand),
  },
  run: async () => {
    // Default action: show list
    const { listCommand } = await import("./list.js");
    await runCommand(listCommand, { rawArgs: [] });
  },
});
