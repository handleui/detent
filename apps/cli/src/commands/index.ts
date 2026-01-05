import { defineCommand } from "citty";
import { getVersion } from "../utils/version.js";

export const main = defineCommand({
  meta: {
    name: "detent",
    version: getVersion(),
    description: "Run GitHub Actions locally with enhanced error reporting",
  },
  subCommands: {
    version: () => import("./version.js").then((m) => m.versionCommand),
    config: () => import("./config/index.js").then((m) => m.configCommand),
  },
});
