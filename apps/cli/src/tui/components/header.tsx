import { Box, Text } from "ink";
import { getVersion } from "../../utils/version.js";

interface HeaderProps {
  command: string;
}

export const Header = ({ command }: HeaderProps): JSX.Element => (
  <Box flexDirection="column" marginBottom={1} marginTop={1}>
    <Text>
      detent v{getVersion()} <Text dimColor>'{command}'</Text>
    </Text>
  </Box>
);

export const printHeader = (command: string): void => {
  console.log();
  console.log(`detent v${getVersion()} '${command}'`);
  console.log();
};
