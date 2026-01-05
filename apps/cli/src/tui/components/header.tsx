import { Box, Text } from "ink";
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
};

/**
 * Prints header with cached update check (instant, no network).
 * Cache is populated by `detent update` or periodic checks.
 *
 * NOTE: Update banner temporarily disabled to reduce latency.
 * The update logic in utils/update.ts remains intact for `dt update`.
 */
export const printHeaderWithUpdateCheck = (command: string): void => {
  printHeader(command);
};
