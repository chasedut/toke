import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { WebLinksAddon } from 'xterm-addon-web-links';
import { WebglAddon } from 'xterm-addon-webgl';
import { invoke } from '@tauri-apps/api/core';
import { listen } from '@tauri-apps/api/event';
import { check } from '@tauri-apps/plugin-updater';
import { ask, message } from '@tauri-apps/plugin-dialog';
import { relaunch } from '@tauri-apps/plugin-process';
import 'xterm/css/xterm.css';
import './style.css';

// Create terminal
const term = new Terminal({
  cursorBlink: true,
  fontSize: 14,
  fontFamily: 'Menlo, Monaco, "Courier New", monospace',
  theme: {
    background: '#1c1e26',
    foreground: '#e0e0e0',
    cursor: '#f07178',
    black: '#1c1e26',
    red: '#ef5253',
    green: '#7cc844',
    yellow: '#e4b51c',
    blue: '#52a8ff',
    magenta: '#a37acc',
    cyan: '#52cbb0',
    white: '#c7c7c7',
    brightBlack: '#616161',
    brightRed: '#ef5253',
    brightGreen: '#7cc844',
    brightYellow: '#e4b51c',
    brightBlue: '#52a8ff',
    brightMagenta: '#a37acc',
    brightCyan: '#52cbb0',
    brightWhite: '#ffffff',
  },
  allowProposedApi: true,
});

// Add addons
const fitAddon = new FitAddon();
term.loadAddon(fitAddon);
term.loadAddon(new WebLinksAddon());

// Try to use WebGL renderer for better performance
try {
  const webglAddon = new WebglAddon();
  webglAddon.onContextLoss(() => {
    webglAddon.dispose();
  });
  term.loadAddon(webglAddon);
} catch (e) {
  console.warn('WebGL addon threw an exception during load', e);
}

// Open terminal in DOM
const container = document.getElementById('terminal');
if (container) {
  term.open(container);
  fitAddon.fit();
}

// Handle window resize
window.addEventListener('resize', () => {
  fitAddon.fit();
});

// Send input to backend
term.onData((data) => {
  invoke('write_to_pty', { data });
});

// Handle resize
term.onResize(({ cols, rows }) => {
  invoke('resize_pty', { cols, rows });
});

// Listen for output from backend
listen<string>('pty-output', (event) => {
  term.write(new Uint8Array(event.payload as any));
});

// Start the toke process
async function startToke() {
  try {
    await invoke('start_toke', { 
      cols: term.cols, 
      rows: term.rows 
    });
  } catch (error) {
    console.error('Failed to start toke:', error);
    term.write('\r\n\x1b[31mFailed to start toke: ' + error + '\x1b[0m\r\n');
  }
}

// Check for updates
async function checkForUpdates() {
  try {
    const update = await check();
    if (update) {
      const shouldUpdate = await ask(
        `Update version ${update.version} is available!\n\nRelease notes:\n${update.body}\n\nWould you like to install it now?`,
        {
          title: 'Update Available',
          kind: 'info',
          okLabel: 'Update Now',
          cancelLabel: 'Later'
        }
      );
      
      if (shouldUpdate) {
        term.write('\r\n\x1b[33mDownloading update...\x1b[0m\r\n');
        
        let downloaded = 0;
        let contentLength = 0;
        
        await update.downloadAndInstall((event) => {
          switch (event.event) {
            case 'Started':
              contentLength = event.data.contentLength || 0;
              term.write(`\r\n\x1b[33mDownload started (${(contentLength / 1024 / 1024).toFixed(2)} MB)\x1b[0m\r\n`);
              break;
            case 'Progress':
              downloaded += event.data.chunkLength;
              const progress = contentLength > 0 ? (downloaded / contentLength * 100).toFixed(1) : '0';
              term.write(`\r\x1b[33mProgress: ${progress}%\x1b[0m`);
              break;
            case 'Finished':
              term.write('\r\n\x1b[32mUpdate downloaded successfully!\x1b[0m\r\n');
              break;
          }
        });
        
        await message('Update installed successfully! The application will restart now.', {
          title: 'Update Complete',
          kind: 'info'
        });
        
        await relaunch();
      }
    }
  } catch (error) {
    console.error('Failed to check for updates:', error);
  }
}

// Start toke when ready
window.addEventListener('DOMContentLoaded', () => {
  setTimeout(startToke, 100);
  // Check for updates after app starts
  setTimeout(checkForUpdates, 5000);
});
