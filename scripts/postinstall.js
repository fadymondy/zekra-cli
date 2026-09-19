#!/usr/bin/env node
// postinstall: download the prebuilt `zekra` binary for this platform from the
// matching GitHub release into bin/. Zero runtime deps — Node stdlib only.
const fs = require("fs");
const path = require("path");
const https = require("https");

const pkg = require(path.join(__dirname, "..", "package.json"));
const VERSION = "v" + pkg.version;

const OS = { darwin: "darwin", linux: "linux", win32: "windows" }[process.platform];
const ARCH = { x64: "amd64", arm64: "arm64" }[process.arch];
const EXT = process.platform === "win32" ? ".exe" : "";

if (!OS || !ARCH) {
  console.error(`[zekra] unsupported platform ${process.platform}/${process.arch}.`);
  console.error("[zekra] install from source instead:  go install github.com/fadymondy/zekra-cli@latest");
  process.exit(0); // don't hard-fail the whole npm install
}

const asset = `zekra-${OS}-${ARCH}${EXT}`;
const url = `https://github.com/fadymondy/zekra-cli/releases/download/${VERSION}/${asset}`;
const binDir = path.join(__dirname, "..", "bin");
const dest = path.join(binDir, process.platform === "win32" ? "zekra.exe" : "zekra-bin");

fs.mkdirSync(binDir, { recursive: true });

function download(u, redirects) {
  if (redirects > 10) { fail("too many redirects"); return; }
  https.get(u, { headers: { "User-Agent": "zekra-cli-postinstall" } }, (res) => {
    if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
      res.resume();
      return download(res.headers.location, redirects + 1);
    }
    if (res.statusCode !== 200) { fail(`HTTP ${res.statusCode} for ${u}`); res.resume(); return; }
    const tmp = dest + ".download";
    const out = fs.createWriteStream(tmp);
    res.pipe(out);
    out.on("finish", () => out.close(() => {
      fs.renameSync(tmp, dest);
      if (process.platform !== "win32") fs.chmodSync(dest, 0o755);
      console.log(`[zekra] installed ${asset} → ${dest}`);
    }));
  }).on("error", (e) => fail(e.message));
}

function fail(msg) {
  console.error(`[zekra] could not download the binary: ${msg}`);
  console.error(`[zekra] you can install manually:  go install github.com/fadymondy/zekra-cli@latest`);
  console.error(`[zekra] or grab ${asset} from https://github.com/fadymondy/zekra-cli/releases/${VERSION}`);
  // Exit 0 so a download hiccup doesn't abort the user's whole `npm install`.
  process.exit(0);
}

download(url, 0);
