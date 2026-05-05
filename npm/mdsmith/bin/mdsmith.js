#!/usr/bin/env node
// mdsmith npm shim — resolves the prebuilt binary from the platform
// optional-dependency that npm installed alongside this package, then
// execs it with the user's argv. No postinstall hook runs, so the
// shim works in offline / air-gapped CI and on hosts that ban network
// calls during npm install. The Go binary embeds the same tag at
// build time, so `mdsmith version` reports the published version on
// every channel.

"use strict";

const PLATFORM_PACKAGES = Object.freeze({
  "linux-x64": "@mdsmith/linux-x64",
  "linux-arm64": "@mdsmith/linux-arm64",
  "darwin-x64": "@mdsmith/darwin-x64",
  "darwin-arm64": "@mdsmith/darwin-arm64",
  "win32-x64": "@mdsmith/win32-x64",
});

function platformPackage(platform, arch) {
  return PLATFORM_PACKAGES[`${platform}-${arch}`];
}

function binaryRelativePath(platform) {
  return `bin/mdsmith${platform === "win32" ? ".exe" : ""}`;
}

function unsupportedMessage(platform, arch) {
  const supported = Object.keys(PLATFORM_PACKAGES).join(", ");
  return (
    `mdsmith: unsupported platform ${platform}-${arch}. ` +
    `Supported: ${supported}. ` +
    `Direct downloads are available at ` +
    `https://github.com/jeduden/mdsmith/releases.`
  );
}

function resolveBinary(platform, arch, resolve) {
  const pkg = platformPackage(platform, arch);
  if (!pkg) {
    const err = new Error(unsupportedMessage(platform, arch));
    err.code = "MDSMITH_UNSUPPORTED_PLATFORM";
    throw err;
  }
  try {
    return resolve(`${pkg}/${binaryRelativePath(platform)}`);
  } catch (cause) {
    // optionalDependencies install best-effort, so a host whose
    // platform-arch combo wasn't picked up by npm sees a clear
    // diagnostic instead of a require-resolve stack trace.
    const err = new Error(
      `mdsmith: missing platform package ${pkg}. ` +
      `Reinstall with --force or download a binary from ` +
      `https://github.com/jeduden/mdsmith/releases.`
    );
    err.code = "MDSMITH_PLATFORM_PACKAGE_MISSING";
    err.cause = cause;
    throw err;
  }
}

function main() {
  const { spawnSync } = require("child_process");
  let bin;
  try {
    bin = resolveBinary(process.platform, process.arch, require.resolve);
  } catch (err) {
    process.stderr.write(`${err.message}\n`);
    process.exit(1);
    return;
  }
  const result = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
  if (result.error) {
    process.stderr.write(`mdsmith: ${result.error.message}\n`);
    process.exit(1);
    return;
  }
  // spawnSync sets `signal` instead of `status` when the child was
  // killed; mirror node-style exit codes (128 + signum) so users can
  // chain mdsmith inside a shell pipeline that distinguishes signals
  // from natural exits.
  if (result.signal) {
    process.exit(128);
    return;
  }
  process.exit(result.status === null ? 1 : result.status);
}

module.exports = {
  PLATFORM_PACKAGES,
  platformPackage,
  binaryRelativePath,
  resolveBinary,
};

if (require.main === module) {
  main();
}
