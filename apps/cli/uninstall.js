#!/usr/bin/env node

const { existsSync, rmSync } = require("node:fs");
const { join } = require("node:path");

const PACKAGE_JSON = require("./package.json");

const uninstall = () => {
  const binDir = join(__dirname, PACKAGE_JSON.goBinary.path);

  if (existsSync(binDir)) {
    rmSync(binDir, { recursive: true, force: true });
    console.log("Binary uninstalled successfully");
  }
};

uninstall();
