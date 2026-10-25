#!/usr/bin/env node
// ESP-17 preview/accessibility probe. It drives headless Chrome over the
// DevTools Protocol against the isolated local HTTPS preview (or an HTTP
// rehearsal), checking metadata, landmarks, labels, no-JS navigation, keyboard
// focus, large-font mobile reflow, same-origin resources and screenshots.
import { spawn, execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";

const SCHEMA = "abastevo-portal-preview-v1";

function parseArgs(argv) {
  const args = {
    chrome: "google-chrome",
    station: "00000000-0000-4000-8000-000000000001",
    insecure: false,
    expectJs: null,
    selfTest: false,
  };
  for (let i = 0; i < argv.length; i += 1) {
    const flag = argv[i];
    if (flag === "--self-test") args.selfTest = true;
    else if (flag === "--insecure") args.insecure = true;
    else if (flag === "--origin") args.origin = argv[++i];
    else if (flag === "--station") args.station = argv[++i];
    else if (flag === "--out") args.out = argv[++i];
    else if (flag === "--chrome") args.chrome = argv[++i];
    else if (flag === "--screenshots") args.screenshots = argv[++i];
    else if (flag === "--expect-js") args.expectJs = argv[++i] === "true";
    else throw new Error(`unknown flag ${flag}`);
  }
  return args;
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
      for (const handler of this.listeners.get(message.method) || []) handler(message.params || {});
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

  once(method, timeoutMs = 20000) {
    return new Promise((resolve, reject) => {
      const handler = (params) => {
        clearTimeout(timer);
        this.listeners.set(method, (this.listeners.get(method) || []).filter((item) => item !== handler));
        resolve(params);
      };
      const timer = setTimeout(() => {
        this.listeners.set(method, (this.listeners.get(method) || []).filter((item) => item !== handler));
        reject(new Error(`timeout waiting for ${method}`));
      }, timeoutMs);
      this.on(method, handler);
    });
  }
}

function launchArgs(profile, insecure) {
  const args = [
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
  ];
  if (insecure) args.push("--ignore-certificate-errors");
  args.push("about:blank");
  return args;
}

async function startChrome(chrome, insecure) {
  const profile = mkdtempSync(join(tmpdir(), "abastevo-preview-"));
  const child = spawn(chrome, launchArgs(profile, insecure), { stdio: ["ignore", "ignore", "pipe"] });
  let stderr = "";
  child.stderr.on("data", (chunk) => { stderr += chunk.toString(); });
  const portFile = join(profile, "DevToolsActivePort");
  for (let i = 0; i < 100; i += 1) {
    if (existsSync(portFile)) break;
    await sleep(100);
  }
  if (!existsSync(portFile)) {
    child.kill("SIGTERM");
    throw new Error(`chrome did not expose DevTools: ${stderr.slice(0, 400)}`);
  }
  const port = Number(readFileSync(portFile, "utf8").split("\n")[0].trim());
  for (let i = 0; i < 50; i += 1) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/version`);
      if (response.ok) break;
    } catch { /* retry */ }
    await sleep(100);
  }
  return { child, profile, port, stderr: () => stderr };
}

async function connectTarget(port) {
  const response = await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, { method: "PUT" });
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
    expression, returnByValue: true, awaitPromise: true,
  });
  if (result.exceptionDetails) throw new Error(`page evaluation failed: ${result.exceptionDetails.text}`);
  return result.result.value;
}

function sameOrigin(resource, origin) {
  try { return new URL(resource).origin === origin; } catch { return false; }
}

function apiPath(resource) {
  try {
    const path = /^https?:/.test(resource) ? new URL(resource).pathname : resource;
    return path.startsWith("/v1/") || path.startsWith("/health/");
  } catch { return false; }
}

function accessibleName(element) {
  return (
    element.innerText ||
    element.getAttribute("aria-label") ||
    element.getAttribute("title") ||
    element.getAttribute("value") ||
    (element.querySelector("img") ? element.querySelector("img").alt : "") ||
    ""
  ).trim();
}

const MARKERS = {
  "/consultar/": ["Consultar preços", "Amostra", "não é preço ao vivo"],
  "/precos/": ["Preços", "não é preço ao vivo"],
  "/precos/uf/SP/": ["São Paulo", "Amostra"],
  "/metodologia/": ["Metodologia", "precisão", "Fonte"],
  "/dados/": ["Baixar dados públicos", "UTF-8", "direitos"],
  "/fiscalizacao/": ["Fiscalização", "não é certidão negativa", "histórico de propriedade"],
};

function markersFor(page) {
  if (page.startsWith("/postos/")) {
    return ["Posto Alfa", "Fonte:", "checksum:", "não é preço ao vivo"];
  }
  return MARKERS[page] || [];
}

async function navigate(cdp, url) {
  const loaded = cdp.once("Page.loadEventFired", 20000);
  await cdp.send("Page.navigate", { url });
  await loaded;
  await sleep(300);
}

async function pageSnapshot(cdp, origin, page, expectJs) {
  const data = await evaluate(cdp, `(() => {
    const links = [...document.querySelectorAll("a[href]")].map((a) => ({
      text: (a.innerText || a.getAttribute("aria-label") || "").trim(),
      href: a.getAttribute("href"),
      className: a.className,
    }));
    const controls = [...document.querySelectorAll("input,select,textarea,button")].map((el) => {
      const labelText = el.labels && el.labels.length
        ? [...el.labels].map((label) => label.innerText).join(" ")
        : "";
      return {
        tag: el.tagName.toLowerCase(), type: el.getAttribute("type") || "",
        name: (labelText || el.getAttribute("aria-label") || el.getAttribute("aria-labelledby") || el.getAttribute("title") || el.innerText || el.getAttribute("value") || "").trim(),
        disabled: !!el.disabled,
      };
    });
    const scripts = [...document.querySelectorAll("script[src]")].map((script) => script.getAttribute("src"));
    const resources = performance.getEntriesByType("resource")
      .map((entry) => entry.name)
      .filter((name) => /^https?:/.test(name));
    const h1 = [...document.querySelectorAll("h1")].map((node) => node.innerText.trim());
    const headings = [...document.querySelectorAll("h1,h2,h3,h4,h5,h6")].map((node) => Number(node.tagName.slice(1)));
    const nav = [...document.querySelectorAll("nav a")].map((node) => node.innerText.trim());
    const skip = document.querySelector("a.skip");
    const main = document.querySelector("main#conteudo");
    return {
      text: document.body ? document.body.innerText : "",
      links, controls, scripts, resources, h1, headings, nav,
      skip: skip ? { href: skip.getAttribute("href"), text: skip.innerText.trim() } : null,
      main: !!main,
      lang: document.documentElement.lang,
      landmarks: {
        header: !!document.querySelector("header"),
        nav: !!document.querySelector("nav"),
        main: !!main,
        footer: !!document.querySelector("footer"),
      },
      imagesWithoutAlt: [...document.images].filter((img) => !img.hasAttribute("alt")).map((img) => img.getAttribute("src")),
    };
  })()`);
  const markers = markersFor(page);
  const missingMarkers = markers.filter((marker) => !data.text.includes(marker));
  const emptyLinks = data.links.filter((link) => !link.text).map((link) => link.href);
  const unnamedControls = data.controls.filter((control) => !control.name).map((control) => `${control.tag}:${control.type}`);
  const external = data.resources.filter((name) => !sameOrigin(name, origin));
  const api = data.resources.filter((name) => apiPath(name));
  const scriptsPresent = data.scripts.length > 0;
  const errors = [];
  if (data.lang !== "pt-BR") errors.push(`lang=${data.lang}`);
  if (data.h1.length !== 1) errors.push(`h1 count=${data.h1.length}`);
  if (!data.main) errors.push("main#conteudo missing");
  if (!data.skip || data.skip.href !== "#conteudo" || !data.skip.text) errors.push("skip link missing");
  if (!data.landmarks.header || !data.landmarks.nav || !data.landmarks.main || !data.landmarks.footer) errors.push("landmark missing");
  if (data.imagesWithoutAlt.length) errors.push(`images without alt=${data.imagesWithoutAlt.length}`);
  if (missingMarkers.length) errors.push(`markers missing=${missingMarkers.join(",")}`);
  if (emptyLinks.length) errors.push(`empty links=${emptyLinks.join(",")}`);
  if (unnamedControls.length) errors.push(`unnamed controls=${unnamedControls.join(",")}`);
  if (external.length) errors.push(`external resources=${external.length}`);
  if (api.length) errors.push(`API resources=${api.length}`);
  if (expectJs === true && !scriptsPresent) errors.push("expected portal script missing");
  if (expectJs === false && scriptsPresent) errors.push("unexpected portal script present");
  return {
    page, url: origin + page, errors, scripts: data.scripts,
    markers: markers.map((marker) => ({ marker, present: data.text.includes(marker) })),
    empty_links: emptyLinks, unnamed_controls: unnamedControls,
    external_resources: external, api_resources: api,
    headings: data.headings, h1: data.h1, nav: data.nav,
    landmarks: data.landmarks, images_without_alt: data.imagesWithoutAlt,
  };
}

async function screenshot(cdp, path) {
  const shot = await cdp.send("Page.captureScreenshot", { format: "png", captureBeyondViewport: false });
  writeFileSync(path, Buffer.from(shot.data, "base64"), { mode: 0o600 });
}

async function keyboardCheck(cdp, origin) {
  await cdp.send("Emulation.setDeviceMetricsOverride", {
    width: 1366, height: 768, deviceScaleFactor: 1, mobile: false,
  });
  await navigate(cdp, origin + "/consultar/");
  // Clear focus, then tab through the page.
  await evaluate(cdp, "document.body.focus(); document.activeElement && document.activeElement.blur();");
  const focus = [];
  for (let i = 0; i < 10; i += 1) {
    await cdp.send("Input.dispatchKeyEvent", {
      type: "keyDown", key: "Tab", code: "Tab", windowsVirtualKeyCode: 9, nativeVirtualKeyCode: 9,
    });
    await cdp.send("Input.dispatchKeyEvent", {
      type: "keyUp", key: "Tab", code: "Tab", windowsVirtualKeyCode: 9, nativeVirtualKeyCode: 9,
    });
    const active = await evaluate(cdp, `(() => {
      const el = document.activeElement;
      if (!el || el === document.body) return null;
      const style = getComputedStyle(el);
      return {
        tag: el.tagName.toLowerCase(), text: (el.innerText || el.getAttribute("aria-label") || "").trim().slice(0, 80),
        href: el.getAttribute("href"), className: el.className,
        focusVisible: el.matches(":focus-visible"),
        outlineStyle: style.outlineStyle, outlineWidth: style.outlineWidth, boxShadow: style.boxShadow,
      };
    })()`);
    if (active) focus.push(active);
  }
  const failures = [];
  if (!focus.length || !String(focus[0].className).includes("skip") || focus[0].href !== "#conteudo") {
    failures.push("first Tab focus is not the skip link");
  }
  const distinct = new Set(focus.map((item) => `${item.tag}:${item.href}:${item.text}`));
  if (distinct.size < 6) failures.push(`only ${distinct.size} distinct focus stops`);
  const noIndicator = focus.filter((item) => item.outlineStyle === "none" && (!item.boxShadow || item.boxShadow === "none"));
  const warnings = noIndicator.length ? [`${noIndicator.length} focus stops have no visible outline/box-shadow`] : [];
  return { focus, failures, warnings };
}

async function noScriptCheck(cdp, origin, station) {
  await cdp.send("Emulation.setScriptExecutionDisabled", { value: true });
  await cdp.send("Emulation.setDeviceMetricsOverride", {
    width: 1366, height: 768, deviceScaleFactor: 1, mobile: false,
  });
  await navigate(cdp, origin + "/consultar/");
  const consultar = await evaluate(cdp, `(() => {
    const noscript = document.querySelector("noscript");
    const links = [...document.querySelectorAll('a[href^="/precos/uf/"]')].map((a) => a.getAttribute("href"));
    return { noscript: noscript ? noscript.innerText : "", stateLinks: links,
      text: document.body ? document.body.innerText : "" };
  })()`);
  await navigate(cdp, origin + "/postos/" + station + "/");
  const stationPage = await evaluate(cdp, `(() => ({
    h1: [...document.querySelectorAll("h1")].map((n) => n.innerText.trim()),
    text: document.body ? document.body.innerText : ""
  }))()`);
  const statePage = consultar.stateLinks.length
    ? await (async () => {
        await navigate(cdp, origin + consultar.stateLinks[0]);
        return evaluate(cdp, `(() => ({
          h1: [...document.querySelectorAll("h1")].map((n) => n.innerText.trim()),
          text: document.body ? document.body.innerText : ""
        }))()`);
      })()
    : { h1: [], text: "" };
  const failures = [];
  if (!consultar.noscript || !consultar.noscript.trim()) failures.push("noscript fallback missing");
  if (consultar.stateLinks.length < 2) failures.push(`state directory links=${consultar.stateLinks.length}`);
  if (!consultar.text.includes("Consultar preços")) failures.push("consultar heading missing without JS");
  if (stationPage.h1.length !== 1 || !stationPage.text.includes("Posto Alfa")) failures.push("station page fails without JS");
  if (statePage.h1.length !== 1 || !statePage.text.includes("postos")) failures.push("state directory page fails without JS");
  return { consultar, station: stationPage, state: statePage, failures };
}

async function largeFontCheck(cdp, origin, station, screenshots) {
  await cdp.send("Emulation.setScriptExecutionDisabled", { value: false });
  await cdp.send("Emulation.setDeviceMetricsOverride", {
    width: 390, height: 844, deviceScaleFactor: 3, mobile: true,
  });
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: 4 });
  const pages = ["/consultar/", "/precos/", "/postos/" + station + "/", "/fiscalizacao/", "/metodologia/", "/dados/"];
  const rows = [];
  const failures = [];
  const warnings = [];
  for (const page of pages) {
    await navigate(cdp, origin + page);
    const result = await evaluate(cdp, `(() => {
      document.documentElement.style.fontSize = "200%";
      const h1 = document.querySelector("h1");
      const rect = h1 ? h1.getBoundingClientRect() : null;
      const targets = [...document.querySelectorAll("nav a, form button, form input, form select, .station-list a")].map((el) => {
        const box = el.getBoundingClientRect();
        return { text: (el.innerText || el.getAttribute("aria-label") || "").trim().slice(0, 50), width: box.width, height: box.height };
      });
      return {
        scrollWidth: document.documentElement.scrollWidth,
        innerWidth: window.innerWidth,
        h1: h1 ? { text: h1.innerText.trim(), width: rect.width, height: rect.height } : null,
        smallTargets: targets.filter((item) => item.width < 24 || item.height < 24),
      };
    })()`);
    const overflow = result.scrollWidth > result.innerWidth + 2;
    if (overflow) failures.push(`${page} horizontal overflow ${result.scrollWidth} > ${result.innerWidth}`);
    if (!result.h1 || result.h1.height === 0) failures.push(`${page} h1 not visible at 200% font`);
    if (result.smallTargets.length) warnings.push(`${page} small primary targets=${result.smallTargets.length}`);
    rows.push({ page, ...result });
    if (screenshots) {
      await screenshot(cdp, join(screenshots, `mobile-large-font${page.replaceAll("/", "_")}.png`));
    }
  }
  return { pages: rows, failures, warnings };
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.selfTest) {
    const assert = (condition, message) => { if (!condition) throw new Error(message); };
    assert(apiPath("/v1/x") && apiPath("https://localhost:1/health/live"), "api paths");
    assert(!apiPath("/consultar/"), "portal path");
    console.log("preview self-test ok");
    return 0;
  }
  if (!args.origin || !args.out) throw new Error("--origin and --out are required");
  const origin = new URL(args.origin).origin;
  if (args.screenshots) {
    mkdirSync(args.screenshots, { recursive: true, mode: 0o700 });
  }
  const pages = [
    "/consultar/", "/precos/", "/precos/uf/SP/",
    `/postos/${args.station}/`, "/fiscalizacao/", "/metodologia/", "/dados/",
  ];
  const chrome = await startChrome(args.chrome, args.insecure);
  let cdp;
  const errors = [];
  try {
    cdp = await connectTarget(chrome.port);
    await cdp.send("Page.enable");
    await cdp.send("Runtime.enable");
    await cdp.send("Network.enable");
    await cdp.send("Log.enable");
    await cdp.send("Accessibility.enable");
    await cdp.send("Emulation.setScriptExecutionDisabled", { value: false });
    await cdp.send("Emulation.setDeviceMetricsOverride", {
      width: 1366, height: 768, deviceScaleFactor: 1, mobile: false,
    });
    cdp.on("Runtime.exceptionThrown", (params) => {
      errors.push(`exception: ${params.exceptionDetails?.text || "unknown"}`);
    });
    cdp.on("Log.entryAdded", (params) => {
      const entry = params.entry || {};
      if (entry.level !== "error" || entry.source !== "javascript") return;
      errors.push(`log: ${entry.text || "error"}`);
    });
    const pageResults = [];
    for (const page of pages) {
      await navigate(cdp, origin + page);
      const result = await pageSnapshot(cdp, origin, page, args.expectJs);
      if (errors.length) result.errors.push(...new Set(errors));
      errors.length = 0;
      pageResults.push(result);
      if (args.screenshots) {
        const name = page.replaceAll("/", "_");
        await screenshot(cdp, join(args.screenshots, `desktop${name}.png`));
      }
    }
    const noScript = await noScriptCheck(cdp, origin, args.station);
    if (args.screenshots) await screenshot(cdp, join(args.screenshots, "noscript_consultar.png"));
    const keyboard = await keyboardCheck(cdp, origin);
    const largeFont = await largeFontCheck(cdp, origin, args.station, args.screenshots);
    const failures = [
      ...pageResults.flatMap((result) => result.errors.map((error) => `${result.page}: ${error}`)),
      ...noScript.failures.map((error) => `no-js: ${error}`),
      ...keyboard.failures.map((error) => `keyboard: ${error}`),
      ...largeFont.failures.map((error) => `large-font: ${error}`),
    ];
    const warnings = [
      ...keyboard.warnings.map((warning) => `keyboard: ${warning}`),
      ...largeFont.warnings.map((warning) => `large-font: ${warning}`),
    ];
    const report = {
      schema_version: SCHEMA,
      chrome: execFileSync(args.chrome, ["--version"], { encoding: "utf8" }).trim(),
      origin,
      artifact: { expect_js: args.expectJs },
      pages: pageResults,
      no_script: noScript,
      keyboard,
      large_font: largeFont,
      failures,
      warnings,
    };
    writeFileSync(args.out, `${JSON.stringify(report, null, 2)}\n`, { mode: 0o600 });
    for (const result of pageResults) {
      console.log(`preview ${result.page} errors=${result.errors.length} markers=${result.markers.filter((m) => m.present).length}/${result.markers.length}`);
    }
    console.log(`preview no-js failures=${noScript.failures.length} keyboard failures=${keyboard.failures.length} large-font failures=${largeFont.failures.length} warnings=${warnings.length}`);
    if (failures.length) {
      for (const failure of failures) console.error(`FAIL: ${failure}`);
      return 1;
    }
    console.log("preview probe PASS");
    return 0;
  } finally {
    try { if (cdp) cdp.ws.close(); } catch { /* best effort */ }
    chrome.child.kill("SIGTERM");
    await sleep(300);
    rmSync(chrome.profile, { recursive: true, force: true });
  }
}

main().then((code) => process.exit(code)).catch((error) => {
  console.error(`preview probe: ${error.message}`);
  process.exit(1);
});
