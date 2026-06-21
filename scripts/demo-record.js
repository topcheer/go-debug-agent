const { chromium } = require('playwright');
const fs = require('fs');
const { execSync } = require('child_process');

/**
 * Go Debug Agent — Full demo recording (25 tools / 8 inspectors)
 *
 * 6 sections using NATURAL LANGUAGE prompts (no explicit tool names).
 * The LLM must autonomously decide which tools to invoke.
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

// ─── Section 1: Go Runtime + Memory ────────────────────────────────────────
// Tools: get_memory_stats, trigger_gc, get_runtime_info, get_gc_stats,
//        get_alloc_stats, get_mem_stats, get_cpu_profile

async function section1_runtime(page) {
  console.log('  [1/6] Go Runtime + Memory Deep Dive');
  await typeMessage(page, "My Go app feels sluggish. Can you check the overall runtime health — memory usage, GC stats, and the Go version we're running?");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Show me detailed memory allocation stats — how many mallocs and frees have happened, and what's the total GC pause time?");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Try forcing a garbage collection — I want to see how much memory can be reclaimed.");
  await sendAndWait(page);
  await pause(page, 5000);
}

// ─── Section 2: Goroutines + Build Info ────────────────────────────────────
// Tools: get_goroutine_count, get_goroutine_stacks, get_goroutine_states,
//        get_goroutine_dump, get_build_info, get_module_deps

async function section2_goroutines_build(page) {
  console.log('  [2/6] Goroutines + Build Info');
  await typeMessage(page, "How many goroutines are currently running? Show me the state distribution — how many are running, waiting, or sleeping.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Show me the goroutine stack traces grouped by similarity. Are there any goroutine leaks or unusual patterns?");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "What Go version was this built with? Show me the module dependencies and their versions.");
  await sendAndWait(page);
  await pause(page, 5000);
}

// ─── Section 3: HTTP Request Tracking ──────────────────────────────────────
// Tools: get_recent_requests, get_request_stats, get_slow_requests,
//        get_error_requests

async function section3_http(page) {
  console.log('  [3/6] HTTP Request Tracking');
  await typeMessage(page, "What HTTP requests have come in recently? Show me the request statistics — P50, P95, P99 latency, and error rate.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Which requests were the slowest? Also show me any error requests with 4xx or 5xx status codes.");
  await sendAndWait(page);
  await pause(page, 5000);
}

// ─── Section 4: System Info + Environment ──────────────────────────────────
// Tools: get_system_info, get_process_info, get_disk_usage,
//        get_environment_variables

async function section4_system(page) {
  console.log('  [4/6] System Info + Environment');
  await typeMessage(page, "Give me the system info — hostname, CPU count, GOMAXPROCS. Also check the disk usage on this machine.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "What environment variables are set? Filter for any that start with 'LLM' or 'GO'.");
  await sendAndWait(page);
  await pause(page, 5000);
}

// ─── Section 5: Network + DNS ──────────────────────────────────────────────
// Tools: get_network_stats, get_dns_info

async function section5_network(page) {
  console.log('  [5/6] Network + DNS');
  await typeMessage(page, "Show me the network configuration — local IP addresses, network interfaces, and hostname.");
  await sendAndWait(page);
  await pause(page, 4000);

  await typeMessage(page, "Test DNS resolution for 'google.com' — how long does it take and what IPs does it resolve to?");
  await sendAndWait(page);
  await pause(page, 5000);
}

// ─── Section 6: Comprehensive Debugging ─────────────────────────────────────
// Cross-cutting scenario that exercises multiple inspectors together

async function section6_comprehensive(page) {
  console.log('  [6/6] Comprehensive Debugging Scenario');
  await typeMessage(page, "I'm debugging a performance issue. Give me a comprehensive overview: memory and GC status, goroutine count and states, recent HTTP requests with any errors, and system info — all in one summary.");
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
║  Go Debug Agent — Demo Recording                              ║
║  25 tools / 8 inspectors                                       ║
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
    { name: '01-runtime', fn: section1_runtime },
    { name: '02-goroutines-build', fn: section2_goroutines_build },
    { name: '03-http', fn: section3_http },
    { name: '04-system', fn: section4_system },
    { name: '05-network', fn: section5_network },
    { name: '06-comprehensive', fn: section6_comprehensive },
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
