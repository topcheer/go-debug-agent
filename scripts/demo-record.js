const { chromium } = require('playwright');
const fs = require('fs');
const { execSync } = require('child_process');

/**
 * Go Debug Agent v0.5.0 — Full demo recording (65 tools / 18 inspectors)
 *
 * 10 sections using NATURAL LANGUAGE prompts (no explicit tool names).
 * The LLM must autonomously decide which tools to invoke.
 *
 * New v0.5.0 inspectors: Security, Health, Scheduler, Error Tracking,
 * WebSocket, plus Redis, Gin routes, GORM, Logging, Cache, Outbound HTTP, Metrics.
 *
 * Usage:
 *   1. Start demo: cd go-debug-agent/demo && LLM_API_KEY=... go run .
 *   2. Run: cd scripts && npm install && node demo-record.js
 */

const BASE_URL = process.env.BASE_URL || 'http://localhost:8080';
const OUTPUT_DIR = './demo-recordings';
const VERSION = 'go-v1';

// ─── Helpers ──────────────────────────────────────────────────────────────

async function typeMessage(page, text, charDelay = 8) {
  const input = page.locator('#input');
  await input.click();
  await input.pressSequentially(text, { delay: charDelay });
}

async function waitForAgentIdle(page, timeout = 120000) {
  // Wait for send button to be re-enabled
  try {
    await page.waitForFunction(() => {
      const btn = document.querySelector('#send');
      return btn && !btn.disabled;
    }, { timeout });
  } catch {
    console.log('  Warning: Agent still busy, waiting more...');
    await page.waitForFunction(() => {
      const btn = document.querySelector('#send');
      return btn && !btn.disabled;
    }, { timeout: 60000 }).catch(() => {
      console.log('  Warning: Force proceeding after extended wait');
    });
  }

  // Wait for DOM to stabilize (no new messages for 3s)
  let lastCount = 0;
  let stableTime = 0;
  let maxWait = 15000;
  const interval = 1000;
  while (stableTime < 3000 && maxWait > 0) {
    const count = await page.evaluate(() => document.querySelectorAll('.message, .tool-badge').length);
    if (count === lastCount) {
      stableTime += interval;
    } else {
      lastCount = count;
      stableTime = 0;
    }
    await page.waitForTimeout(interval);
    maxWait -= interval;
  }
  await page.waitForTimeout(1500);
}

async function sendAndWait(page, timeout = 120000) {
  await page.locator('#send').click();
  await waitForAgentIdle(page, timeout);
}

async function pause(page, ms = 3000) {
  await page.waitForTimeout(ms);
}

// ─── Section 1: Runtime Memory + GC + Allocations ───────────────────────────

async function section1_runtime(page) {
  console.log('  [1/10] Runtime Memory + GC + Allocations');
  await typeMessage(page, "My Go app feels sluggish under load. Can you check the overall runtime health — memory usage, GC stats, and the Go version we're running?");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Show me detailed memory allocation stats — how many mallocs and frees have happened, and what's the total GC pause time?");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Try forcing a garbage collection — I want to see how much memory can be reclaimed.");
  await sendAndWait(page);
  await pause(page, 5000);
  console.log('  → Transition: Goroutines + Build Info');
}

// ─── Section 2: Goroutines + Build Info ─────────────────────────────────────

async function section2_goroutines_build(page) {
  console.log('  [2/10] Goroutines + Build Info');
  await typeMessage(page, "How many goroutines are currently running? Show me the state distribution — how many are running, waiting, or sleeping.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Show me the goroutine stack traces grouped by similarity. Are there any goroutine leaks or unusual patterns?");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "What Go version was this built with? Show me the module dependencies and their versions.");
  await sendAndWait(page);
  await pause(page, 5000);
  console.log('  → Transition: HTTP Requests + Gin Routes');
}

// ─── Section 3: HTTP Requests + Gin Routes ──────────────────────────────────

async function section3_http_routes(page) {
  console.log('  [3/10] HTTP Requests + Gin Routes');
  await typeMessage(page, "What API routes does this Gin application expose? List all the registered endpoints with their HTTP methods.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "What HTTP requests have come in recently? Show me the request statistics — P50, P95, P99 latency, and error rate.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Which requests were the slowest? Also show me any error requests with 4xx or 5xx status codes.");
  await sendAndWait(page);
  await pause(page, 5000);
  console.log('  → Transition: Database (GORM) + Redis Pool');
}

// ─── Section 4: Database (GORM) + Redis Pool ────────────────────────────────

async function section4_db_redis(page) {
  console.log('  [4/10] Database (GORM) + Redis Pool');
  await typeMessage(page, "Is there a GORM database connection in this app? Show me the connection pool status — active, idle, and max connections.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Are there any slow database queries logged? I want to see if there are queries taking more than 100ms.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Check the Redis connection pool — how many connections are active, idle, and what's the pool configuration?");
  await sendAndWait(page);
  await pause(page, 5000);
  console.log('  → Transition: Logging + Cache Stats');
}

// ─── Section 5: Logging + Cache Stats ───────────────────────────────────────

async function section5_logging_cache(page) {
  console.log('  [5/10] Logging + Cache Stats');
  await typeMessage(page, "Show me the logging configuration — what log level is set and what loggers are configured? Also show recent log entries if available.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "What's the cache status? Show me cache hit and miss rates, total keys, and memory usage for any in-memory caches.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Are there any queue or background job workers running? Show me the queue status and any pending jobs.");
  await sendAndWait(page);
  await pause(page, 5000);
  console.log('  → Transition: Security (auth config, sessions)');
}

// ─── Section 6: Security (auth config, sessions) ────────────────────────────

async function section6_security(page) {
  console.log('  [6/10] Security (auth config, sessions)');
  await typeMessage(page, "I'm doing a security audit. What authentication configuration is in place? Show me the auth middleware and any JWT or session settings.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Are there any active sessions? Show me session details — how many are active and when they expire.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Check for any potential security issues — are there any environment variables exposing secrets, or any insecure configurations?");
  await sendAndWait(page);
  await pause(page, 5000);
  console.log('  → Transition: Health Checks (DB, Redis, disk)');
}

// ─── Section 7: Health Checks (DB, Redis, disk) ─────────────────────────────

async function section7_health(page) {
  console.log('  [7/10] Health Checks (DB, Redis, disk)');
  await typeMessage(page, "Run a health check on the database connection — is it reachable and responding quickly?");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Check the health of the Redis connection and also show me disk usage — how much space is free?");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Give me an overall readiness summary — are all the critical dependencies (database, Redis, disk) healthy?");
  await sendAndWait(page);
  await pause(page, 5000);
  console.log('  → Transition: Scheduler + Error Tracking');
}

// ─── Section 8: Scheduler + Error Tracking ──────────────────────────────────

async function section8_scheduler_errors(page) {
  console.log('  [8/10] Scheduler + Error Tracking');
  await typeMessage(page, "Are there any scheduled or cron jobs running? Show me the scheduler status and upcoming job executions.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Show me recent errors tracked by the application — any panics, recovered errors, or error-level log entries?");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Are there any WebSocket connections active? Show me connection details and any connection errors.");
  await sendAndWait(page);
  await pause(page, 5000);
  console.log('  → Transition: Outbound HTTP + FD + Metrics');
}

// ─── Section 9: Outbound HTTP + File Descriptors + Metrics ──────────────────

async function section9_outbound_metrics(page) {
  console.log('  [9/10] Outbound HTTP + File Descriptors + Metrics');
  await typeMessage(page, "What outbound HTTP requests has the application made recently? Show me the external API calls with response times.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "How many file descriptors are currently open? Is there any risk of hitting the FD limit?");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Show me the application metrics — request counts, error rates, latency histograms, and any custom metrics that have been registered.");
  await sendAndWait(page);
  await pause(page, 5000);
  console.log('  → Transition: Comprehensive Multi-Tool Debugging');
}

// ─── Section 10: Comprehensive Multi-Tool Debugging ─────────────────────────

async function section10_comprehensive(page) {
  console.log('  [10/10] Comprehensive Multi-Tool Debugging');
  await typeMessage(page, "I'm investigating a production incident. Give me a comprehensive overview: memory and GC status, goroutine count and states, recent HTTP requests with any errors, database and Redis pool health, and any recent errors — all in one summary.");
  await sendAndWait(page);
  await pause(page, 6000);

  await typeMessage(page, "Based on everything you've found, are there any red flags or areas of concern? What would you recommend I investigate further?");
  await sendAndWait(page);
  await pause(page, 5000);
}

// ─── Main ─────────────────────────────────────────────────────────────────

(async () => {
  console.log(`
╔══════════════════════════════════════════════════════════════╗
║  Go Debug Agent v0.5.0 — Demo Recording                        ║
║  65 tools / 18 inspectors                                       ║
╚══════════════════════════════════════════════════════════════╝
  `);

  if (!fs.existsSync(OUTPUT_DIR)) fs.mkdirSync(OUTPUT_DIR, { recursive: true });

  // Verify app is running
  console.log(`Checking app at ${BASE_URL}/agent ...`);
  try {
    const resp = await fetch(`${BASE_URL}/agent/api/tools`);
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const data = await resp.json();
    console.log(`  Found ${data.tools.length} tools registered`);
  } catch (e) {
    console.error(`ERROR: Demo app not running at ${BASE_URL}. Start it first:\n  cd demo && LLM_API_KEY=... go run .`);
    process.exit(1);
  }

  const browser = await chromium.launch({ headless: false });
  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 },
    recordVideo: { dir: OUTPUT_DIR, size: { width: 1280, height: 800 } },
  });
  const page = await context.newPage();

  console.log(`Navigating to ${BASE_URL}/agent ...`);
  await page.goto(`${BASE_URL}/agent`);
  await pause(page, 2000);

  // Pre-generate some HTTP traffic for request tracking demos
  console.log('Generating HTTP traffic for demos...');
  const endpoints = [
    '/api/orders', '/api/orders/1', '/api/health',
    '/api/slow', '/api/error', '/api/orders',
    '/api/orders/1', '/api/health',
    '/api/orders/2', '/api/error', '/api/slow',
  ];
  for (const ep of endpoints) {
    try { await fetch(`${BASE_URL}${ep}`); } catch {}
  }

  await pause(page, 1000);

  const sections = [
    { name: '01-runtime-memory-gc', fn: section1_runtime },
    { name: '02-goroutines-build', fn: section2_goroutines_build },
    { name: '03-http-routes', fn: section3_http_routes },
    { name: '04-db-redis', fn: section4_db_redis },
    { name: '05-logging-cache', fn: section5_logging_cache },
    { name: '06-security', fn: section6_security },
    { name: '07-health-checks', fn: section7_health },
    { name: '08-scheduler-errors', fn: section8_scheduler_errors },
    { name: '09-outbound-fd-metrics', fn: section9_outbound_metrics },
    { name: '10-comprehensive', fn: section10_comprehensive },
  ];

  const startTime = Date.now();

  for (let i = 0; i < sections.length; i++) {
    const section = sections[i];
    const elapsed = ((Date.now() - startTime) / 60000).toFixed(1);
    console.log(`\n--- [${i + 1}/${sections.length}] ${section.name} (elapsed: ${elapsed} min) ---`);
    await section.fn(page);
    await page.screenshot({ path: `${OUTPUT_DIR}/${VERSION}-demo-${section.name}.png`, fullPage: true });
    console.log(`  Screenshot: ${VERSION}-demo-${section.name}.png`);
  }

  await pause(page, 3000);
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await pause(page, 2000);

  const video = page.video();
  const videoPath = await video.path();
  console.log(`\n  Video path: ${videoPath}`);

  await context.close();
  await browser.close();

  // Rename and convert video
  console.log('\n--- Finalizing video ---');
  const finalWebm = `${OUTPUT_DIR}/${VERSION}-full-demo.webm`;
  const finalMp4 = `${OUTPUT_DIR}/${VERSION}-full-demo.mp4`;

  try { fs.unlinkSync(finalWebm); } catch {}
  try { fs.unlinkSync(finalMp4); } catch {}

  if (videoPath && fs.existsSync(videoPath)) {
    fs.copyFileSync(videoPath, finalWebm);
    const size = fs.statSync(finalWebm).size;
    console.log(`  Saved: ${VERSION}-full-demo.webm (${(size / 1024 / 1024).toFixed(1)} MB)`);
  }

  // Convert to mp4
  try {
    console.log('\n--- Converting to mp4 ---');
    if (fs.existsSync(finalWebm)) {
      execSync(`ffmpeg -y -i "${finalWebm}" -c:v libx264 -preset fast -crf 23 -c:a aac "${finalMp4}"`, { stdio: 'pipe' });
      const size = fs.statSync(finalMp4).size;
      console.log(`  Done: ${VERSION}-full-demo.mp4 (${(size / 1024 / 1024).toFixed(1)} MB)`);
    }
  } catch (e) {
    console.log('  (ffmpeg conversion failed, keeping .webm)');
  }

  const totalMin = ((Date.now() - startTime) / 60000).toFixed(1);
  console.log(`
======================================================
  Recording complete!
  Total time: ${totalMin} minutes
  Output: ${OUTPUT_DIR}/${VERSION}-full-demo.mp4
======================================================
  `);
})();
