#!/usr/bin/env node
// Bounded headless-Chrome probe for the static portal. It drives Chrome through
// the DevTools Protocol (Node's built-in WebSocket, no package dependency) and
// records navigation/paint/LCP/CLS/transfer and JS-heap metrics. It never
// touches the API, database or source. This is a local drill, not a CDN/VPS
// capacity claim.
import { spawn, execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";

const SCHEMA = "abastevo-portal-browser-v1";

function parseArgs(argv) {
  const args = {
    runs: 3,
    chrome: "google-chrome",
    viewports: "desktop,mobile",
    station: "00000000-0000-4000-8000-000000000001",
    selfTest: false,
  };
  for (let i = 0; i < argv.length; i += 1) {
    const flag = argv[i];
    if (flag === "--self-test") {
      args.selfTest = true;
    } else if (flag === "--origin") {
      args.origin = argv[++i];
    } else if (flag === "--station") {
      args.station = argv[++i];
    } else if (flag === "--runs") {
      args.runs = Number(argv[++i]);
    } else if (flag === "--out") {
      args.out = argv[++i];
    } else if (flag === "--chrome") {
      args.chrome = argv[++i];
    } else if (flag === "--viewports") {
      args.viewports = argv[++i];
    } else if (flag === "--pages") {
      args.pages = argv[++i];
    } else {
      throw new Error(`unknown flag ${flag}`);
    }
  }
  return args;
}

function median(values) {
  if (!values.length) return null;
  const ordered = [...values].sort((a, b) => a - b);
  const mid = Math.floor(ordered.length / 2);
  if (ordered.length % 2 === 1) return ordered[mid];
  return (ordered[mid - 1] + ordered[mid]) / 2;
}

function sameOrigin(resource, origin) {
  try {
    return new URL(resource).origin === origin;
  } catch {
    return false;
  }
}

function apiResource(resource) {
  try {
    const path = /^https?:/.test(resource) ? new URL(resource).pathname : resource;
    return path.startsWith("/v1/") || path.startsWith("/health/");
  } catch {
    return false;
  }
}

class CDP {
  constructor(ws) {
    this.ws = ws;
    this.nextId = 1;
    this.pending = new Map();
    this.listeners = new Map();
    ws.addEventListener("message", (event) => {
      const message = JSON.parse(event.data);
      if (message.id && this.pending.has(message.id)) {
        const { resolve, reject } = this.pending.get(message.id);
        this.pending.delete(message.id);
        if (message.error) reject(new Error(message.error.message));
        else resolve(message.result || {});
        return;
      }
      for (const handler of this.listeners.get(message.method) || []) {
        handler(message.params || {});
      }
    });
  }

  send(method, params = {}) {
    const id = this.nextId++;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.ws.send(JSON.stringify({ id, method, params }));
    });
  }

  on(method, handler) {
    if (!this.listeners.has(method)) this.listeners.set(method, []);
    this.listeners.get(method).push(handler);
  }

  once(method, timeoutMs = 15000) {
    return new Promise((resolve, reject) => {
      const handler = (params) => {
        clearTimeout(timer);
        const list = this.listeners.get(method) || [];
        this.listeners.set(method, list.filter((item) => item !== handler));
        resolve(params);
      };
      const timer = setTimeout(() => {
        const list = this.listeners.get(method) || [];
        this.listeners.set(method, list.filter((item) => item !== handler));
        reject(new Error(`timeout waiting for ${method}`));
      }, timeoutMs);
      this.on(method, handler);
    });
  }
}

function chromeLaunchArgs(profile) {
  return [
    "--headless=new",
    "--no-sandbox",
    "--disable-gpu",
    "--disable-background-networking",
    "--disable-component-update",
    "--disable-sync",
    "--disable-features=OptimizationHints,MediaRouter,Translate",
    "--no-first-run",
    "--no-default-browser-check",
    "--hide-scrollbars",
    "--remote-debugging-port=0",
    "--remote-allow-origins=*",
    `--user-data-dir=${profile}`,
    "about:blank",
  ];
}

async function startChrome(chrome) {
  const profile = mkdtempSync(join(tmpdir(), "abastevo-browser-"));
  const child = spawn(chrome, chromeLaunchArgs(profile), {
    stdio: ["ignore", "ignore", "pipe"],
  });
  let stderr = "";
  child.stderr.on("data", (chunk) => { stderr += chunk.toString(); });
  const portFile = join(profile, "DevToolsActivePort");
  for (let i = 0; i < 100; i += 1) {
    if (existsSync(portFile)) break;
    await sleep(100);
  }
  if (!existsSync(portFile)) {
    child.kill("SIGTERM");
    throw new Error(`chrome did not expose DevTools: ${stderr.slice(0, 500)}`);
  }
  const port = Number(readFileSync(portFile, "utf8").split("\n")[0].trim());
  for (let i = 0; i < 50; i += 1) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/version`);
      if (response.ok) break;
    } catch {
      // retry while Chrome finishes booting
    }
    await sleep(100);
  }
  return { child, profile, port, stderr: () => stderr };
}

async function connectTarget(port) {
  const response = await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, {
    method: "PUT",
  });
  if (!response.ok) throw new Error(`cannot create browser target: ${response.status}`);
  const target = await response.json();
  const ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => {
    ws.addEventListener("open", resolve, { once: true });
    ws.addEventListener("error", () => reject(new Error("browser websocket failed")), { once: true });
  });
  return new CDP(ws);
}

async function evaluate(cdp, expression) {
  const result = await cdp.send("Runtime.evaluate", {
    expression,
    returnByValue: true,
    awaitPromise: true,
  });
  if (result.exceptionDetails) {
    throw new Error(`page evaluation failed: ${result.exceptionDetails.text}`);
  }
  return result.result.value;
}

const OBSERVER_SCRIPT = `
  window.__lcp = [];
  window.__cls = 0;
  new PerformanceObserver((list) => {
    for (const entry of list.getEntries()) {
      window.__lcp.push({ startTime: entry.startTime, size: entry.size,
        element: entry.element ? entry.element.tagName : null });
    }
  }).observe({ type: "largest-contentful-paint", buffered: true });
  new PerformanceObserver((list) => {
    for (const entry of list.getEntries()) {
      if (!entry.hadRecentInput) window.__cls += entry.value;
    }
  }).observe({ type: "layout-shift", buffered: true });
`;

const VIEWPORTS = {
  desktop: { width: 1366, height: 768, dpr: 1, mobile: false, cpu: 1 },
  mobile: { width: 390, height: 844, dpr: 3, mobile: true, cpu: 4 },
};

async function measure(cdp, url, viewport, origin, errors) {
  errors.length = 0;
  await cdp.send("Network.clearBrowserCache");
  await cdp.send("Network.setCacheDisabled", { cacheDisabled: true });
  await cdp.send("Emulation.setDeviceMetricsOverride", {
    width: viewport.width,
    height: viewport.height,
    deviceScaleFactor: viewport.dpr,
    mobile: viewport.mobile,
  });
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: viewport.cpu });
  const loaded = cdp.once("Page.loadEventFired", 20000);
  await cdp.send("Page.navigate", { url });
  await loaded;
  await sleep(800);

  const nav = await evaluate(cdp, "performance.getEntriesByType('navigation')[0] ? performance.getEntriesByType('navigation')[0].toJSON() : null");
  if (!nav) throw new Error("navigation timing unavailable");
  const paints = await evaluate(cdp, "performance.getEntriesByType('paint').map((entry) => ({ name: entry.name, startTime: entry.startTime }))");
  const lcpEntries = await evaluate(cdp, "window.__lcp || []");
  const cls = Number(await evaluate(cdp, "window.__cls || 0"));
  const resources = await evaluate(cdp, `performance.getEntriesByType("resource").map((entry) => ({
    name: entry.name, initiatorType: entry.initiatorType,
    transferSize: entry.transferSize || 0, encodedBodySize: entry.encodedBodySize || 0,
    duration: entry.duration
  }))`);
  const perfMetrics = await cdp.send("Performance.getMetrics");
  const metric = (name) => {
    const found = (perfMetrics.metrics || []).find((item) => item.name === name);
    return found ? found.value : null;
  };
  const fcp = paints.find((entry) => entry.name === "first-contentful-paint");
  const lcp = lcpEntries.length
    ? lcpEntries.reduce((best, entry) => (entry.startTime > best.startTime ? entry : best))
    : null;
  const httpResources = resources.filter((entry) => {
    try { return ["http:", "https:"].includes(new URL(entry.name).protocol); } catch { return false; }
  });
  const external = httpResources.filter((entry) => !sameOrigin(entry.name, origin)).map((entry) => entry.name);
  const api = httpResources.filter((entry) => apiResource(entry.name)).map((entry) => entry.name);
  const transferBytes = (nav.transferSize || 0) +
    httpResources.reduce((sum, entry) => sum + entry.transferSize, 0);
  return {
    url,
    ttfb_ms: nav.responseStart,
    fcp_ms: fcp ? fcp.startTime : null,
    lcp_ms: lcp ? lcp.startTime : null,
    cls,
    load_ms: nav.loadEventEnd || nav.duration,
    dom_content_loaded_ms: nav.domContentLoadedEventEnd,
    requests: httpResources.length + 1,
    transfer_bytes: transferBytes,
    js_heap_bytes: metric("JSHeapUsedSize"),
    nodes: metric("Nodes"),
    layout_count: metric("LayoutCount"),
    external_resources: [...new Set(external)],
    api_resources: [...new Set(api)],
    errors: [...errors],
  };
}

function summarize(runs, pages, viewports) {
  const summary = [];
  for (const page of pages) {
    for (const viewport of viewports) {
      const rows = runs.filter((row) => row.page === page && row.viewport === viewport);
      if (!rows.length) continue;
      const values = (key) => rows.map((row) => row[key]).filter((value) => value !== null && value !== undefined);
      summary.push({
        page,
        viewport,
        runs: rows.length,
        ttfb_median_ms: median(values("ttfb_ms")),
        fcp_median_ms: median(values("fcp_ms")),
        lcp_median_ms: median(values("lcp_ms")),
        cls_median: median(values("cls")),
        load_median_ms: median(values("load_ms")),
        transfer_bytes_median: median(values("transfer_bytes")),
        requests_median: median(values("requests")),
        js_heap_bytes_median: median(values("js_heap_bytes")),
        errors: rows.reduce((sum, row) => sum + row.errors.length, 0),
        external_resources: [...new Set(rows.flatMap((row) => row.external_resources))],
        api_resources: [...new Set(rows.flatMap((row) => row.api_resources))],
        lcp_unavailable: rows.some((row) => row.lcp_ms === null),
        fcp_unavailable: rows.some((row) => row.fcp_ms === null),
      });
    }
  }
  return summary;
}

function selfTest() {
  const assert = (condition, message) => {
    if (!condition) throw new Error(message);
  };
  assert(median([1, 2, 3]) === 2, "median odd");
  assert(median([1, 2, 3, 4]) === 2.5, "median even");
  assert(sameOrigin("http://127.0.0.1:1/a", "http://127.0.0.1:1"), "same origin");
  assert(!sameOrigin("https://example.test/a", "http://127.0.0.1:1"), "cross origin");
  assert(apiResource("/v1/health/live"), "api path");
  assert(!apiResource("/consultar/"), "non-api path");
  console.log("browser self-test ok");
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.selfTest) {
    selfTest();
    return 0;
  }
  if (!args.origin || !args.out) throw new Error("--origin and --out are required");
  if (!Number.isInteger(args.runs) || args.runs < 1 || args.runs > 10) {
    throw new Error("--runs must be 1..10");
  }
  const origin = new URL(args.origin).origin;
  const pages = args.pages
    ? args.pages.split(",").map((page) => page.trim()).filter(Boolean)
    : ["/consultar/", `/postos/${args.station}/`, "/fiscalizacao/"];
  const viewports = args.viewports.split(",").map((name) => name.trim()).filter(Boolean);
  for (const name of viewports) {
    if (!VIEWPORTS[name]) throw new Error(`unknown viewport ${name}`);
  }
  const chrome = await startChrome(args.chrome);
  let cdp;
  const errors = [];
  const runs = [];
  try {
    cdp = await connectTarget(chrome.port);
    await cdp.send("Page.enable");
    await cdp.send("Runtime.enable");
    await cdp.send("Network.enable");
    await cdp.send("Log.enable");
    await cdp.send("Performance.enable");
    await cdp.send("Page.addScriptToEvaluateOnNewDocument", { source: OBSERVER_SCRIPT });
    cdp.on("Runtime.exceptionThrown", (params) => {
      errors.push(`exception: ${params.exceptionDetails?.text || "unknown"}`);
    });
    cdp.on("Log.entryAdded", (params) => {
      const entry = params.entry || {};
      if (entry.level !== "error" || entry.source !== "javascript") return;
      if (entry.url && !sameOrigin(entry.url, origin)) return;
      errors.push(`log: ${entry.text || "error"}`);
    });
    for (const viewportName of viewports) {
      for (const page of pages) {
        const url = origin + page;
        for (let run = 1; run <= args.runs; run += 1) {
          const row = await measure(cdp, url, VIEWPORTS[viewportName], origin, errors);
          runs.push({ page, viewport: viewportName, run, ...row });
        }
      }
    }
    const version = execFileSync(args.chrome, ["--version"], { encoding: "utf8" }).trim();
    const result = {
      schema_version: SCHEMA,
      chrome: version,
      origin,
      viewports,
      pages,
      runs,
      summary: summarize(runs, pages, viewports),
    };
    writeFileSync(args.out, `${JSON.stringify(result, null, 2)}\n`, { mode: 0o600 });
    for (const row of result.summary) {
      console.log(
        `browser ${row.viewport} ${row.page} fcp=${row.fcp_median_ms} lcp=${row.lcp_median_ms} ` +
        `cls=${row.cls_median} ttfb=${row.ttfb_median_ms} bytes=${row.transfer_bytes_median} ` +
        `errors=${row.errors} external=${row.external_resources.length} api=${row.api_resources.length}`);
    }
    return 0;
  } finally {
    try { if (cdp) cdp.ws.close(); } catch { /* best effort */ }
    chrome.child.kill("SIGTERM");
    await sleep(300);
    rmSync(chrome.profile, { recursive: true, force: true });
  }
}

main().then((code) => process.exit(code)).catch((error) => {
  console.error(`browser probe: ${error.message}`);
  process.exit(1);
});
