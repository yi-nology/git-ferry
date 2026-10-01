#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const BINARY_NAME = "gitferry";

function getBinaryName(platform = process.platform) {
  return platform === "win32" ? `${BINARY_NAME}.exe` : BINARY_NAME;
}

function getBinaryPath(platform = process.platform, baseDir = __dirname) {
  return path.join(baseDir, getBinaryName(platform));
}

function formatMissingBinaryError(binaryPath, platform = process.platform, arch = process.arch) {
  return [
    `Error: ${BINARY_NAME} binary not found at ${binaryPath}`,
    `Platform: ${platform}/${arch}`,
    "",
    "The npm package was installed, but the native binary is missing.",
    "This usually means the release asset for your platform is unavailable or postinstall failed.",
    "",
    "Try reinstalling:",
    "  npm install -g gitferry-cli",
    "",
    "Or download from GitHub Releases:",
    "  https://github.com/yi-nology/git-ferry/releases",
    "",
    "Or build from source:",
    "  make build-cli",
  ].join("\n");
}

function run(args = process.argv.slice(2), options = {}) {
  const platform = options.platform || process.platform;
  const arch = options.arch || process.arch;
  const binaryPath = options.binaryPath || getBinaryPath(platform);
  const execFile = options.execFileSync || execFileSync;
  const stderr = options.stderr || process.stderr;
  const exit = options.exit || process.exit;

  if (!fs.existsSync(binaryPath)) {
    stderr.write(`${formatMissingBinaryError(binaryPath, platform, arch)}\n`);
    return exit(1);
  }

  try {
    execFile(binaryPath, args, { stdio: "inherit" });
  } catch (err) {
    if (err.code === "ENOENT") {
      stderr.write(`${formatMissingBinaryError(binaryPath, platform, arch)}\n`);
      return exit(1);
    }
    if (err.status !== undefined) {
      return exit(err.status);
    }
    stderr.write(`Failed to run ${BINARY_NAME}: ${err.message}\n`);
    return exit(1);
  }
}

if (require.main === module) {
  run();
}

module.exports = {
  BINARY_NAME,
  getBinaryName,
  getBinaryPath,
  formatMissingBinaryError,
  run,
};
