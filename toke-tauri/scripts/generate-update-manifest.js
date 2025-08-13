#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const crypto = require('crypto');

// Read the Tauri config to get version
const tauriConfig = JSON.parse(
  fs.readFileSync(path.join(__dirname, '../src-tauri/tauri.conf.json'), 'utf8')
);

const version = tauriConfig.version;
const platforms = {
  'darwin-aarch64': {
    signature: '',
    url: `https://github.com/chasedut/toke/releases/download/v${version}/Toke_${version}_aarch64.app.tar.gz`,
  },
  'darwin-x86_64': {
    signature: '',
    url: `https://github.com/chasedut/toke/releases/download/v${version}/Toke_${version}_x64.app.tar.gz`,
  },
  'windows-x86_64': {
    signature: '',
    url: `https://github.com/chasedut/toke/releases/download/v${version}/Toke_${version}_x64-setup.nsis.zip`,
  }
};

// Read signatures if they exist
const bundleDir = path.join(__dirname, '../src-tauri/target/release/bundle');
if (fs.existsSync(bundleDir)) {
  const macosDir = path.join(bundleDir, 'macos');
  if (fs.existsSync(macosDir)) {
    // Read ARM64 signature
    const arm64SigPath = path.join(macosDir, `Toke_${version}_aarch64.app.tar.gz.sig`);
    if (fs.existsSync(arm64SigPath)) {
      platforms['darwin-aarch64'].signature = fs.readFileSync(arm64SigPath, 'utf8').trim();
    }
    
    // Read x64 signature
    const x64SigPath = path.join(macosDir, `Toke_${version}_x64.app.tar.gz.sig`);
    if (fs.existsSync(x64SigPath)) {
      platforms['darwin-x86_64'].signature = fs.readFileSync(x64SigPath, 'utf8').trim();
    }
  }
  
  const nsisDir = path.join(bundleDir, 'nsis');
  if (fs.existsSync(nsisDir)) {
    const winSigPath = path.join(nsisDir, `Toke_${version}_x64-setup.nsis.zip.sig`);
    if (fs.existsSync(winSigPath)) {
      platforms['windows-x86_64'].signature = fs.readFileSync(winSigPath, 'utf8').trim();
    }
  }
}

const updateManifest = {
  version,
  pub_date: new Date().toISOString(),
  platforms
};

// Write the update manifest
fs.writeFileSync(
  path.join(__dirname, '../latest.json'),
  JSON.stringify(updateManifest, null, 2)
);

console.log('Update manifest generated:', updateManifest);