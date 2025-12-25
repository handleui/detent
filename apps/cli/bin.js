#!/usr/bin/env node

const { spawn } = require("node:child_process");
const { join } = require("node:path");

const binPath = join(__dirname, "bin", "dt");

const child = spawn(binPath, process.argv.slice(2), { stdio: "inherit" });

child.on("exit", (code) => {
  process.exit(code);
});
