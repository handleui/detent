import { defineCommand } from "citty";
import { getVersion } from "../utils/version.js";

export const versionCommand = defineCommand({
  meta: {
    name: "version",
    description: "Show detent version",
  },
  run: () => {
    console.log(getVersion());
  },
});
