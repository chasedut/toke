#!/usr/bin/env node

const ngrok = require('ngrok');
const dotenv = require('dotenv');

// Load environment variables
dotenv.config();

const DEFAULT_PORT = process.env.NGROK_PORT || 3000;
const AUTH_TOKEN = process.env.NGROK_AUTH_TOKEN;

async function startTunnel() {
  try {
    // Configure ngrok with auth token if provided
    if (AUTH_TOKEN) {
      await ngrok.authtoken(AUTH_TOKEN);
    }

    // Start ngrok tunnel
    const url = await ngrok.connect({
      addr: DEFAULT_PORT,
      region: process.env.NGROK_REGION || 'us',
      subdomain: process.env.NGROK_SUBDOMAIN,
    });

    console.log(`🚀 Ngrok tunnel established!`);
    console.log(`📡 Public URL: ${url}`);
    console.log(`🔌 Forwarding to: http://localhost:${DEFAULT_PORT}`);
    
    // Keep the process running
    process.stdin.resume();
    
    // Graceful shutdown
    process.on('SIGINT', async () => {
      console.log('\n⏹️  Shutting down ngrok tunnel...');
      await ngrok.kill();
      process.exit(0);
    });

  } catch (error) {
    console.error('❌ Failed to start ngrok tunnel:', error.message);
    process.exit(1);
  }
}

// Start the tunnel
startTunnel();