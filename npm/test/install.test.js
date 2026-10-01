"use strict";

const assert = require("assert");
const { getPlatformInfo, getBinaryName, getArchiveName, BINARY_NAME, VERSION } = require("../scripts/install.js");

function testPlatformInfo() {
  const info = getPlatformInfo("darwin", "arm64");
  assert.strictEqual(info.platform, "darwin");
  assert.strictEqual(info.arch, "arm64");
  assert.strictEqual(info.isWindows, false);

  const win = getPlatformInfo("win32", "x64");
  assert.strictEqual(win.platform, "windows");
  assert.strictEqual(win.arch, "amd64");
  assert.strictEqual(win.isWindows, true);
  console.log("ok platform info");
}

function testArchiveName() {
  assert.strictEqual(getBinaryName("linux"), "gitferry");
  assert.strictEqual(getBinaryName("windows"), "gitferry.exe");
  const a = getArchiveName("linux", "amd64");
  assert.ok(a.includes(BINARY_NAME));
  assert.ok(a.includes(VERSION));
  assert.ok(a.endsWith(".tar.gz"));
  const z = getArchiveName("windows", "amd64");
  assert.ok(z.endsWith(".zip"));
  console.log("ok archive names");
}

function testUnsupported() {
  assert.throws(() => getPlatformInfo("sunos", "x64"));
  console.log("ok unsupported platform");
}

testPlatformInfo();
testArchiveName();
testUnsupported();
console.log("install.test.js passed");
