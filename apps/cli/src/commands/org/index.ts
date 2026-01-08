import { defineCommand, runCommand } from "citty";

export const orgCommand = defineCommand({
  meta: {
    name: "org",
    description: "Manage organizations",
  },
  subCommands: {
    install: () => import("./install.js").then((m) => m.installCommand),
    list: () => import("./list.js").then((m) => m.listCommand),
    status: () => import("./status.js").then((m) => m.statusCommand),
    members: () => import("./members.js").then((m) => m.membersCommand),
    leave: () => import("./leave.js").then((m) => m.leaveCommand),
  },
  run: async () => {
    // Default action: show list
    const { listCommand } = await import("./list.js");
    await runCommand(listCommand, { rawArgs: [] });
  },
});
