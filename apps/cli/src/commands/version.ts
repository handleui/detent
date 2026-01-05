import { defineCommand } from "citty";
import { printHeader } from "../tui/components/index.js";
import { getVersion } from "../utils/version.js";

export const versionCommand = defineCommand({
  meta: {
    name: "version",
    description: "Show detent version",
  },
  run: () => {
    printHeader("version");
    console.log(`v${getVersion()}`);
    console.log();
  },
});
