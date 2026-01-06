import { findGitRoot } from "@detent/git";
import {
  ensureRepoDetentDir,
  formatBudget,
  type GlobalConfig,
  getAllowedModels,
  loadRepoConfig,
  maskApiKey,
  saveRepoConfig,
  validateApiKey,
} from "../../lib/config.js";

const MODELS = getAllowedModels();

const validateApiKeyInput = (
  value: string
): { valid: boolean; error?: string } => {
  if (!value || value.trim() === "") {
    return { valid: true };
  }
  return validateApiKey(value);
};

import { defineCommand } from "citty";
import { Box, render, Text, useApp, useInput } from "ink";
import { useCallback, useEffect, useRef, useState } from "react";
import { Header } from "../../tui/components/index.js";
import { shouldUseTUI } from "../../tui/render.js";
import { colors } from "../../tui/styles.js";
import type { ConfigKey } from "./constants.js";

type FieldType = "text" | "model" | "number";

interface FieldConfig {
  key: ConfigKey;
  label: string;
  type: FieldType;
  format?: (value: unknown) => string;
  isEmpty?: (value: unknown) => boolean;
  min?: number;
  max?: number;
  defaultValue?: unknown;
}

const isEmptyValue = (value: unknown): boolean => {
  if (value === undefined || value === null) {
    return true;
  }
  if (value === "") {
    return true;
  }
  return false;
};

const isEmptyApiKey = (value: unknown): boolean => {
  if (isEmptyValue(value)) {
    return true;
  }
  const str = String(value);
  return str.length === 0 || str === "undefined";
};

const FIELDS: FieldConfig[] = [
  {
    key: "apiKey",
    label: "API Key",
    type: "text",
    format: (v) => {
      if (isEmptyApiKey(v)) {
        return "not set";
      }
      return maskApiKey(String(v));
    },
    isEmpty: isEmptyApiKey,
    defaultValue: undefined,
  },
  {
    key: "model",
    label: "Model",
    type: "model",
    format: (v) => {
      if (isEmptyValue(v)) {
        return "default";
      }
      return String(v);
    },
    isEmpty: isEmptyValue,
    defaultValue: MODELS[0],
  },
  {
    key: "budgetPerRunUsd",
    label: "Budget/Run",
    type: "number",
    format: (v) => {
      const num = Number(v);
      if (Number.isNaN(num) || num === 0) {
        return "unlimited";
      }
      return formatBudget(num);
    },
    isEmpty: (v) => isEmptyValue(v) || Number(v) === 0,
    min: 0,
    max: 100,
    defaultValue: 0,
  },
  {
    key: "budgetMonthlyUsd",
    label: "Budget/Month",
    type: "number",
    format: (v) => {
      const num = Number(v);
      if (Number.isNaN(num) || num === 0) {
        return "unlimited";
      }
      return formatBudget(num);
    },
    isEmpty: (v) => isEmptyValue(v) || Number(v) === 0,
    min: 0,
    max: 1000,
    defaultValue: 0,
  },
  {
    key: "timeoutMins",
    label: "Timeout/Run",
    type: "number",
    format: (v) => {
      const num = Number(v);
      if (Number.isNaN(num) || num === 0) {
        return "none";
      }
      return `${num} min`;
    },
    isEmpty: (v) => isEmptyValue(v) || Number(v) === 0,
    min: 0,
    max: 60,
    defaultValue: 0,
  },
];

const LABEL_WIDTH = 16;
const DIGIT_REGEX = /^[0-9]$/;
const NUMBER_INPUT_REGEX = /^[0-9.]$/;

const getDisplayValue = (
  field: FieldConfig,
  value: unknown,
  isFocused: boolean,
  isEditing: boolean,
  editValue: string
): string => {
  if (isFocused && isEditing) {
    if (field.type === "text") {
      return editValue || "_";
    }
    if (field.type === "number") {
      return editValue || "_";
    }
  }
  if (field.format) {
    return field.format(value);
  }
  return String(value ?? "");
};

const getHelp = (field: FieldConfig, isEditing: boolean): string => {
  if (field.type === "text") {
    return isEditing ? "enter save, esc cancel" : "type or paste";
  }
  if (field.type === "model") {
    return "← → cycle";
  }
  if (field.type === "number") {
    const bounds =
      field.min !== undefined && field.max !== undefined
        ? ` (${field.min}-${field.max})`
        : "";
    return isEditing ? "enter save, esc cancel" : `← → ±1 • shift ±10${bounds}`;
  }
  return "";
};

const hasFieldChanged = (
  field: FieldConfig,
  originalValue: unknown,
  currentValue: unknown
): boolean => {
  // Handle empty/undefined values
  const origEmpty = field.isEmpty
    ? field.isEmpty(originalValue)
    : isEmptyValue(originalValue);
  const currEmpty = field.isEmpty
    ? field.isEmpty(currentValue)
    : isEmptyValue(currentValue);

  // If both empty, no change
  if (origEmpty && currEmpty) {
    return false;
  }

  // If one is empty and the other isn't, it changed
  if (origEmpty !== currEmpty) {
    return true;
  }

  // Compare actual values
  return String(originalValue) !== String(currentValue);
};

interface ConfigEditorProps {
  repoRoot: string;
}

export const ConfigEditor = ({ repoRoot }: ConfigEditorProps): JSX.Element => {
  const { exit } = useApp();
  const originalConfig = useRef<GlobalConfig>(loadRepoConfig(repoRoot));
  const [draftConfig, setDraftConfig] = useState<GlobalConfig>(() =>
    loadRepoConfig(repoRoot)
  );
  const [focusIndex, setFocusIndex] = useState(0);
  const [isEditing, setIsEditing] = useState(false);
  const [editValue, setEditValue] = useState("");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [hasChanges, setHasChanges] = useState(false);
  const [isExiting, setIsExiting] = useState(false);
  const exitErrorRef = useRef<string | null>(null);

  useEffect(() => {
    if (errorMessage) {
      const timer = setTimeout(() => setErrorMessage(null), 3000);
      return () => clearTimeout(timer);
    }
  }, [errorMessage]);

  const handleUpdate = useCallback((key: ConfigKey, value: unknown) => {
    setDraftConfig((prev) => ({
      ...prev,
      [key]: value,
    }));
    setHasChanges(true);
  }, []);

  const handleClear = useCallback(
    (field: FieldConfig) => {
      let clearValue: unknown;
      if (field.type === "number") {
        clearValue = 0;
      } else if (field.type === "model") {
        clearValue = "";
      } else if (field.type === "text") {
        clearValue = undefined;
      }
      handleUpdate(field.key, clearValue);
    },
    [handleUpdate]
  );

  const handleTextEditing = useCallback(
    (
      field: FieldConfig,
      input: string,
      key: {
        return: boolean;
        escape: boolean;
        backspace: boolean;
        delete: boolean;
        ctrl: boolean;
        meta: boolean;
      }
    ): boolean => {
      if (key.return) {
        if (field.key === "apiKey") {
          const validation = validateApiKeyInput(editValue);
          if (!validation.valid) {
            setErrorMessage(validation.error ?? "Invalid API key format");
            return true;
          }
        }
        handleUpdate(field.key, editValue || undefined);
        setIsEditing(false);
        setErrorMessage(null);
        return true;
      }
      if (key.escape) {
        setErrorMessage(null);
        setIsEditing(false);
        return true;
      }
      if (key.backspace || key.delete) {
        setEditValue((v) => v.slice(0, -1));
        return true;
      }
      if (input && !key.ctrl && !key.meta) {
        setEditValue((v) => v + input);
        return true;
      }
      return false;
    },
    [editValue, handleUpdate]
  );

  const handleNumberEditing = useCallback(
    (
      field: FieldConfig,
      input: string,
      key: {
        return: boolean;
        escape: boolean;
        backspace: boolean;
        delete: boolean;
        ctrl: boolean;
        meta: boolean;
      }
    ): boolean => {
      if (key.return) {
        const num = Number(editValue);
        if (editValue !== "" && !Number.isNaN(num)) {
          const min = field.min ?? 0;
          const max = field.max ?? 999;
          const clamped = Math.max(min, Math.min(max, num));
          handleUpdate(field.key, clamped);
        }
        setIsEditing(false);
        setErrorMessage(null);
        return true;
      }
      if (key.escape) {
        setErrorMessage(null);
        setIsEditing(false);
        return true;
      }
      if (key.backspace || key.delete) {
        setEditValue((v) => v.slice(0, -1));
        return true;
      }
      if (input && !key.ctrl && !key.meta && NUMBER_INPUT_REGEX.test(input)) {
        setEditValue((v) => v + input);
        return true;
      }
      return false;
    },
    [editValue, handleUpdate]
  );

  const handleModelCycle = useCallback(
    (field: FieldConfig, key: { leftArrow: boolean; rightArrow: boolean }) => {
      const currentVal = String(draftConfig[field.key] ?? "");
      let idx = MODELS.indexOf(currentVal);
      if (idx === -1) {
        idx = 0;
      }
      if (key.leftArrow) {
        const newIndex = (idx - 1 + MODELS.length) % MODELS.length;
        handleUpdate(field.key, MODELS[newIndex]);
      } else if (key.rightArrow) {
        const newIndex = (idx + 1) % MODELS.length;
        handleUpdate(field.key, MODELS[newIndex]);
      }
    },
    [draftConfig, handleUpdate]
  );

  const handleNumberAdjust = useCallback(
    (
      field: FieldConfig,
      key: { leftArrow: boolean; rightArrow: boolean; shift: boolean }
    ) => {
      const raw = draftConfig[field.key];
      const num = Number.isNaN(Number(raw)) ? 0 : Number(raw);
      const min = field.min ?? 0;
      const max = field.max ?? 999;
      const step = key.shift ? 10 : 1;
      if (key.leftArrow) {
        handleUpdate(field.key, Math.max(min, num - step));
      } else if (key.rightArrow) {
        handleUpdate(field.key, Math.min(max, num + step));
      }
    },
    [draftConfig, handleUpdate]
  );

  const enterTextEditMode = useCallback(
    (field: FieldConfig) => {
      const raw = draftConfig[field.key];
      setEditValue(isEmptyValue(raw) ? "" : String(raw));
      setIsEditing(true);
    },
    [draftConfig]
  );

  const navigateUp = useCallback(() => {
    setFocusIndex((i) => (i - 1 + FIELDS.length) % FIELDS.length);
  }, []);

  const navigateDown = useCallback(() => {
    setFocusIndex((i) => (i + 1) % FIELDS.length);
  }, []);

  const handleGlobalShortcut = useCallback(
    (field: FieldConfig, input: string): boolean => {
      if (input === "c") {
        handleClear(field);
        return true;
      }
      return false;
    },
    [handleClear]
  );

  const handleExit = useCallback(() => {
    exitErrorRef.current = errorMessage;
    setIsExiting(true);

    // Check if any field actually changed from original
    const hasActualChanges = FIELDS.some((field) =>
      hasFieldChanged(
        field,
        originalConfig.current[field.key],
        draftConfig[field.key]
      )
    );

    if (hasActualChanges && !errorMessage) {
      ensureRepoDetentDir(repoRoot);
      saveRepoConfig(draftConfig, repoRoot);
      setHasChanges(true); // Update state for exit message
    } else {
      setHasChanges(false); // No actual changes
    }

    setTimeout(() => {
      exit();
    }, 0);
  }, [exit, errorMessage, draftConfig, repoRoot]);

  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: Input handler requires comprehensive conditional logic
  useInput((input, key) => {
    const field = FIELDS[focusIndex];
    if (!field) {
      return;
    }

    if (
      !isEditing &&
      (input === "q" || key.escape || (key.ctrl && input === "c"))
    ) {
      handleExit();
      return;
    }

    if (field.type === "text") {
      if (isEditing) {
        handleTextEditing(field, input, key);
        return;
      }
      // Start editing on any input (including paste via Cmd+V)
      if (input && !key.ctrl) {
        enterTextEditMode(field);
        // Process input (single char or pasted text)
        setEditValue(input);
        return;
      }
    }

    if (field.type === "number") {
      if (isEditing) {
        handleNumberEditing(field, input, key);
        return;
      }
      if (input && DIGIT_REGEX.test(input)) {
        setEditValue(input);
        setIsEditing(true);
        return;
      }
      if (key.leftArrow || key.rightArrow) {
        handleNumberAdjust(field, key);
        return;
      }
    }

    if (!isEditing && handleGlobalShortcut(field, input)) {
      return;
    }

    if (key.upArrow) {
      navigateUp();
      return;
    }
    if (key.downArrow) {
      navigateDown();
      return;
    }

    if (field.type === "model") {
      handleModelCycle(field, key);
    }
  });

  if (isExiting) {
    let exitMessage: string;
    let exitIcon: string;
    if (exitErrorRef.current) {
      exitMessage = exitErrorRef.current;
      exitIcon = "✗";
    } else if (hasChanges) {
      exitMessage = "Config updated";
      exitIcon = "✓";
    } else {
      exitMessage = "Dismissed";
      exitIcon = "○";
    }

    return (
      <Box flexDirection="column">
        <Header command="config" />
        <Text color={colors.text}>
          {exitIcon} {exitMessage}
        </Text>
        <Text> </Text>
      </Box>
    );
  }

  return (
    <Box flexDirection="column">
      <Header command="config" />
      <Box marginTop={1} />

      <Box flexDirection="column" marginBottom={1}>
        {FIELDS.map((field, index) => {
          const isFocused = index === focusIndex;
          const value = draftConfig[field.key];
          const empty = field.isEmpty
            ? field.isEmpty(value)
            : isEmptyValue(value);

          const displayValue = getDisplayValue(
            field,
            value,
            isFocused,
            isEditing,
            editValue
          );

          const isFieldEditing = isFocused && isEditing;
          const isChanged = hasFieldChanged(
            field,
            originalConfig.current[field.key],
            value
          );

          // Color logic: grey = empty API key, green = changed or editing, white = unchanged
          let valueColor: string;
          if (isFieldEditing || isChanged) {
            valueColor = colors.brand; // Green when editing or changed
          } else if (empty && field.key === "apiKey") {
            valueColor = colors.muted; // Grey for "not set" API key
          } else {
            valueColor = colors.text; // White for unchanged values
          }

          let indicator = " ";
          if (isFieldEditing) {
            indicator = "✎";
          } else if (isFocused) {
            indicator = "›";
          }

          return (
            <Box gap={2} key={field.key}>
              <Text color={isFocused ? colors.brand : colors.muted}>
                {indicator}
              </Text>
              <Box width={LABEL_WIDTH}>
                <Text color={colors.text}>{field.label}</Text>
              </Box>
              <Text color={valueColor}>{displayValue}</Text>
              {isFocused && (
                <Text color={colors.muted}>{getHelp(field, isEditing)}</Text>
              )}
            </Box>
          );
        })}
      </Box>

      {errorMessage && <Text color="red">{errorMessage}</Text>}
      <Text color={colors.muted}>↑↓ navigate • c clear • q/esc close</Text>
      <Text> </Text>
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

    const repoRoot = await findGitRoot(process.cwd());
    if (!repoRoot) {
      console.error("Error: Not in a git repository.");
      process.exit(1);
    }

    const { waitUntilExit } = render(<ConfigEditor repoRoot={repoRoot} />);
    await waitUntilExit();
  },
});
