import { defineCommand } from "citty";
import { getVersion } from "../utils/version.js";

export const main = defineCommand({
  meta: {
    name: "detent",
    version: getVersion(),
    description: "Run GitHub Actions locally with enhanced error reporting",
  },
  subCommands: {
    init: () => import("./init.js").then((m) => m.initCommand),
    version: () => import("./version.js").then((m) => m.versionCommand),
    config: () => import("./config/index.js").then((m) => m.configCommand),
    auth: () => import("./auth/index.js").then((m) => m.authCommand),
    link: () => import("./link/index.js").then((m) => m.linkCommand),
    mock: () => import("./mock/index.js").then((m) => m.mockCommand),
    jobs: () => import("./mock/jobs.js").then((m) => m.jobsCommand),
    update: () => import("./update.js").then((m) => m.updateCommand),
  },
});
