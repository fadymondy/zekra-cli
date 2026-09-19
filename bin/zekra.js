#!/usr/bin/env node
// Thin launcher: exec the platform binary that postinstall placed next to this file.
const { spawnSync } = require("child_process");
const path = require("path");
const fs = require("fs");

const bin = path.join(__dirname, process.platform === "win32" ? "zekra.exe" : "zekra-bin");

if (!fs.existsSync(bin)) {
  console.error("[zekra] binary not found — the postinstall step may have failed.");
  console.error("[zekra] reinstall (`npm i -g zekra-cli`) or:  go install github.com/fadymondy/zekra-cli@latest");
  process.exit(1);
}

const r = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
if (r.error) { console.error("[zekra]", r.error.message); process.exit(1); }
process.exit(r.status == null ? 1 : r.status);
