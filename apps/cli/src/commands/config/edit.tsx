import {
  type Config,
  formatBudget,
  type GlobalConfig,
  loadConfig,
  loadGlobalConfig,
  maskApiKey,
  saveConfig,
} from "@detent/persistence";
import { Select, TextInput } from "@inkjs/ui";
import { defineCommand } from "citty";
import { Box, render, Text, useApp, useInput } from "ink";
import { useCallback, useState } from "react";
import { Header } from "../../tui/components/index.js";
import { shouldUseTUI } from "../../tui/render.js";
import type { ConfigKey } from "./constants.js";

interface ConfigField {
  key: ConfigKey;
  label: string;
  getValue: (config: Config) => string;
  parse: (value: string) => unknown;
}

const CONFIG_FIELDS: ConfigField[] = [
  {
    key: "apiKey",
    label: "API Key",
    getValue: (config) => maskApiKey(config.apiKey) || "(not set)",
    parse: (value) => value,
  },
  {
    key: "model",
    label: "Model",
    getValue: (config) => config.model,
    parse: (value) => value,
  },
  {
    key: "budgetPerRunUsd",
    label: "Budget per Run",
    getValue: (config) => formatBudget(config.budgetPerRunUsd),
    parse: (value) => Number(value),
  },
  {
    key: "budgetMonthlyUsd",
    label: "Monthly Budget",
    getValue: (config) => formatBudget(config.budgetMonthlyUsd),
    parse: (value) => Number(value),
  },
  {
    key: "timeoutMins",
    label: "Timeout (mins)",
    getValue: (config) => String(config.timeoutMins),
    parse: (value) => Number(value),
  },
];

type EditorState = "selecting" | "editing";

const ConfigEditor = (): JSX.Element => {
  const { exit } = useApp();
  const [config, setConfig] = useState<Config>(loadConfig);
  const [globalConfig, setGlobalConfig] =
    useState<GlobalConfig>(loadGlobalConfig);
  const [state, setState] = useState<EditorState>("selecting");
  const [selectedKey, setSelectedKey] = useState<ConfigKey | null>(null);
  const [editValue, setEditValue] = useState("");
  const [message, setMessage] = useState<string | null>(null);

  const handleSelect = useCallback(
    (value: string) => {
      if (value === "__exit__") {
        exit();
        return;
      }
      const key = value as ConfigKey;
      setSelectedKey(key);
      const field = CONFIG_FIELDS.find((f) => f.key === key);
      if (field) {
        const rawValue = globalConfig[key];
        setEditValue(rawValue !== undefined ? String(rawValue) : "");
      }
      setState("editing");
    },
    [exit, globalConfig]
  );

  const handleSubmit = useCallback(
    (value: string) => {
      if (!selectedKey) {
        return;
      }

      const field = CONFIG_FIELDS.find((f) => f.key === selectedKey);
      if (!field) {
        return;
      }

      try {
        const parsed = field.parse(value);
        const updated: GlobalConfig = {
          ...globalConfig,
          [selectedKey]: parsed,
        };
        saveConfig(updated);
        setGlobalConfig(updated);
        setConfig(loadConfig());
        setMessage(`Saved ${field.label}`);
        setTimeout(() => setMessage(null), 2000);
      } catch {
        setMessage(`Invalid value for ${field.label}`);
        setTimeout(() => setMessage(null), 2000);
      }

      setState("selecting");
      setSelectedKey(null);
      setEditValue("");
    },
    [selectedKey, globalConfig]
  );

  useInput((input, key) => {
    if (state === "editing" && key.escape) {
      setState("selecting");
      setSelectedKey(null);
      setEditValue("");
    }
    if (state === "selecting" && (input === "q" || key.escape)) {
      exit();
    }
  });

  const options = [
    ...CONFIG_FIELDS.map((field) => ({
      label: `${field.label}: ${field.getValue(config)}`,
      value: field.key,
    })),
    { label: "Exit", value: "__exit__" },
  ];

  return (
    <Box flexDirection="column">
      <Header command="config" />

      {state === "selecting" && (
        <Box flexDirection="column">
          <Text dimColor>
            Use arrow keys to select, Enter to edit, q to exit
          </Text>
          <Box marginTop={1}>
            <Select onChange={handleSelect} options={options} />
          </Box>
        </Box>
      )}

      {state === "editing" && selectedKey && (
        <Box flexDirection="column">
          <Text>
            Editing{" "}
            <Text bold>
              {CONFIG_FIELDS.find((f) => f.key === selectedKey)?.label}
            </Text>
            :
          </Text>
          <Box marginTop={1}>
            <TextInput
              defaultValue={editValue}
              onSubmit={handleSubmit}
              placeholder="Enter new value..."
            />
          </Box>
          <Text dimColor>Press Enter to save, Escape to cancel</Text>
        </Box>
      )}

      {message && (
        <Box marginTop={1}>
          <Text color="green">{message}</Text>
        </Box>
      )}

      <Box marginTop={1}>
        <Text dimColor> </Text>
      </Box>
    </Box>
  );
};

export const configEditCommand = defineCommand({
  meta: {
    name: "edit",
    description: "Interactively edit configuration values",
  },
  run: async () => {
    if (!shouldUseTUI()) {
      console.error(
        "Interactive mode requires a TTY. Use 'detent config get <key>' or 'detent config set <key> <value>' for scripting."
      );
      process.exit(1);
    }

    const { waitUntilExit } = render(<ConfigEditor />);
    await waitUntilExit();
  },
});
