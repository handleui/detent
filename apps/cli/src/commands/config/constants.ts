export const CONFIG_KEYS = [
  "apiKey",
  "model",
  "budgetPerRunUsd",
  "budgetMonthlyUsd",
  "timeoutMins",
] as const;

export type ConfigKey = (typeof CONFIG_KEYS)[number];

export const isConfigKey = (key: string): key is ConfigKey => {
  return CONFIG_KEYS.includes(key as ConfigKey);
};
