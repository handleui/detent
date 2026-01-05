import { defineCommand } from "citty";
import { printHeader } from "../tui/components/index.js";
import { ANSI_RESET, colors, hexToAnsi } from "../tui/styles.js";
import { checkForUpdate, runUpdate } from "../utils/update.js";
import { getVersion } from "../utils/version.js";

const brandAnsi = hexToAnsi(colors.brand);
const mutedAnsi = hexToAnsi(colors.muted);
const errorAnsi = hexToAnsi(colors.error);

export const updateCommand = defineCommand({
  meta: {
    name: "update",
    description: "Update detent to the latest version",
  },
  run: async () => {
    printHeader("update");

    const currentVersion = getVersion();
    const { hasUpdate, latestVersion } = await checkForUpdate(currentVersion);

    if (!hasUpdate) {
      console.log(`${brandAnsi}✓${ANSI_RESET} Already on latest version`);
      console.log();
      return;
    }

    console.log(
      `${mutedAnsi}v${currentVersion}${ANSI_RESET} → ${brandAnsi}${latestVersion}${ANSI_RESET}`
    );
    console.log();

    const success = await runUpdate();

    console.log();
    if (success) {
      console.log(`${brandAnsi}✓${ANSI_RESET} Updated successfully`);
    } else {
      console.log(`${errorAnsi}✗${ANSI_RESET} Update failed`);
      process.exit(1);
    }
    console.log();
  },
});
