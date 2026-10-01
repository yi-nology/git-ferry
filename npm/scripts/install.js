#!/usr/bin/env node
"use strict";

/**
 * postinstall：按平台从 GitHub Releases 下载 gitferry 二进制到 npm/bin/。
 * 本地开发 / 离线场景：若 bin/ 下已有二进制则跳过。
 */

const os = require("os");
const path = require("path");
const fs = require("fs");
const https = require("https");
const { execSync } = require("child_process");

const PACKAGE = require("../package.json");
const VERSION = PACKAGE.version;
const BINARY_NAME = "gitferry";
const REPO_OWNER = "yi-nology";
const REPO_NAME = "git-ferry";

function getPlatformInfo(platform = os.platform(), arch = os.arch()) {
  const platformMap = { darwin: "darwin", linux: "linux", win32: "windows" };
  const archMap = { x64: "amd64", arm64: "arm64" };
  const goPlatform = platformMap[platform];
  const goArch = archMap[arch];
  if (!goPlatform || !goArch) {
    throw new Error(
      `Unsupported platform: ${platform}-${arch}. ` +
        `Supported: darwin-x64, darwin-arm64, linux-x64, linux-arm64, win32-x64, win32-arm64`
    );
  }
  return { platform: goPlatform, arch: goArch, isWindows: platform === "win32" };
}

function getBinaryName(platform) {
  return platform === "windows" ? BINARY_NAME + ".exe" : BINARY_NAME;
}

function getArchiveName(platform, arch) {
  const ext = platform === "windows" ? ".zip" : ".tar.gz";
  return `${BINARY_NAME}_${VERSION}_${platform}_${arch}${ext}`;
}

function download(url, dest, maxRedirects = 5) {
  return new Promise((resolve, reject) => {
    const doRequest = (currentUrl, redirects) => {
      https
        .get(currentUrl, (res) => {
          if ([301, 302, 307, 308].includes(res.statusCode) && res.headers.location) {
            if (redirects >= maxRedirects) {
              reject(new Error("Too many redirects"));
              return;
            }
            res.resume();
            doRequest(res.headers.location, redirects + 1);
            return;
          }
          if (res.statusCode !== 200) {
            res.resume();
            reject(new Error(`HTTP ${res.statusCode} for ${currentUrl}`));
            return;
          }
          const file = fs.createWriteStream(dest);
          res.pipe(file);
          file.on("finish", () => file.close(() => resolve(dest)));
          file.on("error", reject);
        })
        .on("error", reject);
    };
    doRequest(url, 0);
  });
}

function extract(archivePath, destDir, isWindows) {
  if (isWindows) {
    execSync(`powershell -Command "Expand-Archive -Path '${archivePath}' -DestinationPath '${destDir}' -Force"`, {
      stdio: "inherit",
    });
    return;
  }
  execSync(`tar -xzf "${archivePath}" -C "${destDir}"`, { stdio: "inherit" });
}

async function main() {
  const binDir = path.join(__dirname, "..", "bin");
  const info = getPlatformInfo();
  const binaryName = getBinaryName(info.platform);
  const binaryPath = path.join(binDir, binaryName);

  // 本地已有二进制（源码构建 / 二次安装）→ 跳过下载
  if (fs.existsSync(binaryPath)) {
    console.log(`gitferry binary already present: ${binaryPath}`);
    return;
  }

  const archiveName = getArchiveName(info.platform, info.arch);
  const url = `https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/v${VERSION}/${archiveName}`;
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "gitferry-"));
  const archivePath = path.join(tmpDir, archiveName);

  console.log(`Downloading gitferry ${VERSION} for ${info.platform}/${info.arch}...`);
  console.log(`  ${url}`);

  try {
    await download(url, archivePath);
    extract(archivePath, tmpDir, info.isWindows);

    const extracted = path.join(tmpDir, binaryName);
    if (!fs.existsSync(extracted)) {
      // 压缩包内可能带一层目录
      const candidates = fs.readdirSync(tmpDir).filter((f) => f.startsWith(BINARY_NAME) && !f.endsWith(".tar.gz") && !f.endsWith(".zip"));
      if (candidates.length === 0) {
        throw new Error(`Binary ${binaryName} not found in archive`);
      }
      fs.copyFileSync(path.join(tmpDir, candidates[0]), binaryPath);
    } else {
      fs.copyFileSync(extracted, binaryPath);
    }
    fs.chmodSync(binaryPath, 0o755);
    console.log(`Installed ${binaryPath}`);
  } catch (err) {
    console.warn(`gitferry postinstall download failed: ${err.message}`);
    console.warn("You can still install manually:");
    console.warn("  1) Download from https://github.com/yi-nology/git-ferry/releases");
    console.warn("  2) Or build from source: make build-cli && cp output/gitferry npm/bin/");
    // 不阻断 npm install：源码/离线场景仍可用
  } finally {
    try {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    } catch (_) {}
  }
}

if (require.main === module) {
  main().catch((err) => {
    console.error(err.message);
    process.exit(1);
  });
}

module.exports = {
  getPlatformInfo,
  getBinaryName,
  getArchiveName,
  BINARY_NAME,
  VERSION,
};
