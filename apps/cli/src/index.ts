#!/usr/bin/env node
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { runMain } from "citty";
import { config } from "dotenv";
import { main } from "./commands/index.js";

// Load .env from CLI package directory (for WORKOS_CLIENT_ID)
const __dirname = dirname(fileURLToPath(import.meta.url));
config({ path: resolve(__dirname, "..", ".env") });

runMain(main);
