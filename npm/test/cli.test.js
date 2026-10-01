"use strict";

const assert = require("assert");
const path = require("path");
const { getBinaryName, getBinaryPath, formatMissingBinaryError } = require("../bin/cli.js");

function testBinaryName() {
  assert.strictEqual(getBinaryName("darwin"), "gitferry");
  assert.strictEqual(getBinaryName("linux"), "gitferry");
  assert.strictEqual(getBinaryName("win32"), "gitferry.exe");
  console.log("ok binary name");
}

function testBinaryPath() {
  const p = getBinaryPath("darwin", "/tmp/bin");
  assert.strictEqual(p, path.join("/tmp/bin", "gitferry"));
  console.log("ok binary path");
}

function testMissingError() {
  const msg = formatMissingBinaryError("/x/gitferry", "linux", "x64");
  assert.ok(msg.includes("binary not found"));
  assert.ok(msg.includes("make build-cli"));
  console.log("ok missing binary message");
}

testBinaryName();
testBinaryPath();
testMissingError();
console.log("cli.test.js passed");
