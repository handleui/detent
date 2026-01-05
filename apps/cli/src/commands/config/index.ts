import { defineCommand } from "citty";
import { configEditCommand } from "./edit.js";
import { configGetCommand } from "./get.js";
import { configListCommand } from "./list.js";
import { configSetCommand } from "./set.js";

export const configCommand = defineCommand({
  meta: {
    name: "config",
    description: "Manage detent configuration",
  },
  subCommands: {
    edit: configEditCommand,
    get: configGetCommand,
    set: configSetCommand,
    list: configListCommand,
  },
  run: async () => {
    const { configEditCommand: editCmd } = await import("./edit.js");
    await editCmd.run?.({ args: { _: [] }, rawArgs: [], cmd: editCmd });
  },
});
