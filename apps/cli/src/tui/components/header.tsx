import { Box, Text } from "ink";
import { checkForUpdateCached } from "../../utils/update.js";
import { getVersion } from "../../utils/version.js";
import { ANSI_RESET, colors, hexToAnsi } from "../styles.js";

interface HeaderProps {
  command: string;
  updateAvailable?: string | null;
}

export const Header = ({
  command,
  updateAvailable,
}: HeaderProps): JSX.Element => (
  <Box flexDirection="column" marginTop={1}>
    <Text>
      <Text color={colors.brand}>Detent v{getVersion()}</Text>{" "}
      <Text color={colors.text}>{command}</Text>
    </Text>
    {updateAvailable ? (
      <>
        <Text color={colors.info}>
          ! {updateAvailable} available · run 'dt update'
        </Text>
        <Text> </Text>
      </>
    ) : null}
  </Box>
);

export const printHeader = (command: string): void => {
  const brandAnsi = hexToAnsi(colors.brand);
  console.log();
  console.log(`${brandAnsi}Detent v${getVersion()}${ANSI_RESET} ${command}`);
  console.log();
};

/**
 * Prints header with cached update check (instant, no network).
 * Cache is populated by `detent update` or periodic checks.
 */
export const printHeaderWithUpdateCheck = (command: string): void => {
  const brandAnsi = hexToAnsi(colors.brand);
  const infoAnsi = hexToAnsi(colors.info);

  const currentVersion = getVersion();
  const result = checkForUpdateCached(currentVersion);

  console.log();
  console.log(`${brandAnsi}Detent v${currentVersion}${ANSI_RESET} ${command}`);

  if (result?.hasUpdate && result.latestVersion) {
    console.log(
      `${infoAnsi}! ${result.latestVersion} available · run 'dt update'${ANSI_RESET}`
    );
  }
  console.log();
};
