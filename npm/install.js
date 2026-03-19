#!/usr/bin/env node

const https = require("https");
const http = require("http");
const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const REPO = "dvflw/mantle";

const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

function getPlatform() {
  const platform = PLATFORM_MAP[process.platform];
  if (!platform) {
    throw new Error(
      `Unsupported platform: ${process.platform}. Mantle supports linux and darwin.`
    );
  }
  return platform;
}

function getArch() {
  const arch = ARCH_MAP[process.arch];
  if (!arch) {
    throw new Error(
      `Unsupported architecture: ${process.arch}. Mantle supports x64 and arm64.`
    );
  }
  return arch;
}

function getVersion() {
  const pkg = require("./package.json");
  return pkg.version;
}

function fetch(url) {
  return new Promise((resolve, reject) => {
    const client = url.startsWith("https") ? https : http;
    client
      .get(url, { headers: { "User-Agent": "dvflw-mantle-npm" } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return fetch(res.headers.location).then(resolve, reject);
        }
        if (res.statusCode !== 200) {
          return reject(
            new Error(`Download failed: HTTP ${res.statusCode} for ${url}`)
          );
        }
        const chunks = [];
        res.on("data", (chunk) => chunks.push(chunk));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      })
      .on("error", reject);
  });
}

function extractTarGz(buffer, destDir) {
  const tarball = path.join(destDir, "_mantle.tar.gz");
  fs.writeFileSync(tarball, buffer);
  try {
    execFileSync("tar", ["xzf", tarball, "-C", destDir], { stdio: "pipe" });
  } finally {
    fs.unlinkSync(tarball);
  }
}

async function main() {
  const platform = getPlatform();
  const arch = getArch();
  const version = getVersion();
  const name = `mantle-${platform}-${arch}`;
  const url = `https://github.com/${REPO}/releases/download/v${version}/${name}.tar.gz`;

  const binDir = path.join(__dirname, "bin");
  const binPath = path.join(binDir, "mantle");

  console.log(`Downloading mantle v${version} for ${platform}/${arch}...`);

  try {
    const buffer = await fetch(url);

    if (!fs.existsSync(binDir)) {
      fs.mkdirSync(binDir, { recursive: true });
    }

    extractTarGz(buffer, binDir);

    // The tarball contains a file named mantle-<os>-<arch>; rename to mantle
    const extracted = path.join(binDir, name);
    if (fs.existsSync(extracted)) {
      fs.renameSync(extracted, binPath);
    }

    fs.chmodSync(binPath, 0o755);

    console.log(`Mantle v${version} installed successfully.`);
  } catch (err) {
    console.error(`Failed to install mantle: ${err.message}`);
    console.error(
      `You can download it manually from https://github.com/${REPO}/releases`
    );
    process.exit(1);
  }
}

main();
