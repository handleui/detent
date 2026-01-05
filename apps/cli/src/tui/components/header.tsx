import { Box, Text } from "ink";
import { getVersion } from "../../utils/version.js";
import { ANSI_RESET, colors, hexToAnsi } from "../styles.js";

interface HeaderProps {
  command: string;
}

export const Header = ({ command }: HeaderProps): JSX.Element => (
  <Box flexDirection="column" marginTop={1}>
    <Text>
      <Text color={colors.brand}>Detent v{getVersion()}</Text>{" "}
      <Text color={colors.text}>{command}</Text>
    </Text>
  </Box>
);

export const printHeader = (command: string): void => {
  const brandAnsi = hexToAnsi(colors.brand);
  console.log();
  console.log(`${brandAnsi}Detent v${getVersion()}${ANSI_RESET} ${command}`);
  console.log();
};
