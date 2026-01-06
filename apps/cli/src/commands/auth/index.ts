import { defineCommand } from "citty";

export const authCommand = defineCommand({
  meta: {
    name: "auth",
    description: "Manage authentication",
  },
  subCommands: {
    login: () => import("./login.js").then((m) => m.loginCommand),
    logout: () => import("./logout.js").then((m) => m.logoutCommand),
    status: () => import("./status.js").then((m) => m.statusCommand),
  },
});
