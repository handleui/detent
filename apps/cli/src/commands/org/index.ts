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
    members: () => import("./members.js").then((m) => m.membersCommand),
    join: () => import("./join.js").then((m) => m.joinCommand),
    leave: () => import("./leave.js").then((m) => m.leaveCommand),
  },
  run: async () => {
    // Default action: show list
    const { listCommand } = await import("./list.js");
    await runCommand(listCommand, { rawArgs: [] });
  },
});
