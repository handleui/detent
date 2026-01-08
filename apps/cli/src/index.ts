#!/usr/bin/env node
import { runMain } from "citty";
import { main } from "./commands/index.js";

// Injected at compile time for standalone binaries
declare const DETENT_PRODUCTION: boolean | undefined;

// Load .env only in development (compiled binaries have env vars baked in)
if (typeof DETENT_PRODUCTION === "undefined") {
  const { dirname, resolve } = await import("node:path");
  const { fileURLToPath } = await import("node:url");
  const { config } = await import("dotenv");
  const __dirname = dirname(fileURLToPath(import.meta.url));
  config({ path: resolve(__dirname, "..", ".env") });
}

runMain(main);
