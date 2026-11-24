// Purpose: take headless-browser pictures of a Network Doctor page for the parity workflows
// (ND-296). It starts the machine's own Chrome, drives it through the DevTools protocol, waits for
// what the page shows (never a fixed sleep), clicks where the plan says, and saves PNG pictures at
// exact sizes, checking each picture's width and height afterwards.
// Owner: Rafiq. Version 0.3.0 (2026-11-20: the fits step; 0.2.0: the touch step; 0.1.0: first build).
//
// Use:  node shot.mjs PLAN.json
// PLAN.json: {"url": "...", "steps": [ ... ]} where each step is one of
//   {"size": [1280, 800]}                       set the window size (1 pixel per CSS pixel)
//   {"wait": "JS expression", "timeout": 120}   wait until the expression is true
//   {"click": "element id"}                     click the element with this id
//   {"touch": "path"}                           write this file, to tell the caller "ready"
//   {"shot": "path/name.png"}                   save a picture of the window
//   {"fits": "what is shown"}                   fail when the page is wider than the window, or
//                                               any shown text runs past its own box (ND-279)
// Chrome is found from the CHROME environment value, else the usual place on each system.
// Needs Node 22 or newer (its built-in WebSocket); nothing is downloaded.

import { spawn } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const plan = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));

function findChrome() {
  const list = [
    process.env.CHROME,
    '/usr/bin/google-chrome',
    '/usr/bin/google-chrome-stable',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
    'C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe',
  ];
  for (const p of list) {
    if (p && fs.existsSync(p)) return p;
  }
  throw new Error('Chrome was not found; set CHROME to its path');
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// The profile folder is new each run, so no saved state changes the pictures.
const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'nd-shot-'));
const chrome = spawn(findChrome(), [
  '--headless=new', '--disable-gpu', '--no-first-run', '--no-default-browser-check',
  '--hide-scrollbars', '--remote-debugging-port=0', `--user-data-dir=${profile}`, 'about:blank',
], { stdio: 'ignore' });

async function devtoolsAddress() {
  const file = path.join(profile, 'DevToolsActivePort');
  for (let i = 0; i < 300; i++) {
    if (fs.existsSync(file)) {
      const [port, p] = fs.readFileSync(file, 'utf8').split('\n');
      if (port && p) return `ws://127.0.0.1:${port.trim()}${p.trim()}`;
    }
    await sleep(100);
  }
  throw new Error('Chrome did not open its DevTools port within 30 seconds');
}

let ws;
let nextId = 1;
const waiting = new Map();

function send(method, params = {}, sessionId) {
  const id = nextId++;
  const msg = { id, method, params };
  if (sessionId) msg.sessionId = sessionId;
  ws.send(JSON.stringify(msg));
  return new Promise((resolve, reject) => {
    waiting.set(id, { resolve, reject, method });
  });
}

function pngSize(file) {
  const b = fs.readFileSync(file);
  // The PNG header's first chunk (IHDR) holds width and height as 4-byte numbers.
  return [b.readUInt32BE(16), b.readUInt32BE(20)];
}

async function main() {
  ws = new WebSocket(await devtoolsAddress());
  await new Promise((resolve, reject) => {
    ws.onopen = resolve;
    ws.onerror = () => reject(new Error('could not connect to Chrome'));
  });
  ws.onmessage = (ev) => {
    const m = JSON.parse(ev.data);
    if (m.id && waiting.has(m.id)) {
      const w = waiting.get(m.id);
      waiting.delete(m.id);
      if (m.error) w.reject(new Error(`${w.method}: ${m.error.message}`));
      else w.resolve(m.result);
    }
  };
  const { targetId } = await send('Target.createTarget', { url: 'about:blank' });
  const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true });
  const s = (method, params) => send(method, params, sessionId);
  await s('Page.enable');
  await s('Runtime.enable');
  let size = [1280, 800];
  const setSize = async (w, h) => {
    size = [w, h];
    await s('Emulation.setDeviceMetricsOverride',
      { width: w, height: h, deviceScaleFactor: 1, mobile: w < 600 });
  };
  await setSize(size[0], size[1]);
  await s('Page.navigate', { url: plan.url });

  const evaluate = async (expr) => {
    let r;
    try {
      r = await s('Runtime.evaluate', { expression: expr, returnByValue: true, awaitPromise: true });
    } catch {
      return undefined; // the page was between documents; the next try sees the new one
    }
    if (r.exceptionDetails) return undefined;
    return r.result.value;
  };

  for (const step of plan.steps) {
    if (step.size) {
      await setSize(step.size[0], step.size[1]);
      // Two animation frames let the page lay itself out at the new width before a picture.
      await evaluate('new Promise(r => requestAnimationFrame(() => requestAnimationFrame(() => r(true))))');
      console.log(`size ${step.size[0]}x${step.size[1]}`);
    } else if (step.wait) {
      const end = Date.now() + (step.timeout || 120) * 1000;
      let ok = false;
      while (Date.now() < end) {
        if (await evaluate(`Boolean(${step.wait})`)) { ok = true; break; }
        await sleep(500);
      }
      if (!ok) throw new Error(`waited ${step.timeout || 120} seconds and this never became true: ${step.wait}`);
      console.log(`seen: ${step.wait}`);
    } else if (step.touch) {
      fs.writeFileSync(step.touch, 'ready\n'); // the caller waits for this file
      console.log(`signalled ${step.touch}`);
    } else if (step.click) {
      const done = await evaluate(
        `(function(){var e=document.getElementById(${JSON.stringify(step.click)});if(!e)return false;e.click();return true;})()`);
      if (!done) throw new Error(`no element with id ${step.click} to click`);
      console.log(`clicked ${step.click}`);
    } else if (step.fits) {
      // The window width is the one this plan set, not innerWidth alone: a phone-sized window can
      // widen innerWidth to fit a page that is too wide. A box that scrolls by itself is allowed.
      const faults = await evaluate(`(function(W){
        var out = [], de = document.documentElement;
        if (de.scrollWidth > W || de.scrollWidth > window.innerWidth) {
          out.push('the page is ' + de.scrollWidth + ' wide in a window ' + W + ' wide');
        }
        document.querySelectorAll('p,h1,h2,li,label,pre,button,td,th,span,div').forEach(function (el) {
          if (!el.getClientRects().length) return;
          var cs = getComputedStyle(el);
          if (cs.overflowX !== 'visible') return;
          var r = el.getBoundingClientRect();
          if (el.scrollWidth > el.clientWidth + 1 && el.clientWidth > 0) {
            out.push(el.tagName.toLowerCase() + (el.id ? '#' + el.id : '') + ' text runs ' +
              (el.scrollWidth - el.clientWidth) + ' past its box: ' + (el.textContent || '').trim().slice(0, 60));
          } else if (r.right > W + 0.5) {
            out.push(el.tagName.toLowerCase() + (el.id ? '#' + el.id : '') + ' ends at ' + Math.round(r.right) +
              ', past the window edge');
          }
        });
        return out.slice(0, 10);
      })(${size[0]})`);
      if (!Array.isArray(faults)) throw new Error(`could not measure the page for "${step.fits}"`);
      if (faults.length) throw new Error(`"${step.fits}" does not fit ${size[0]} wide: ${faults.join('; ')}`);
      console.log(`fits ${size[0]} wide: ${step.fits}`);
    } else if (step.shot) {
      const r = await s('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false });
      fs.mkdirSync(path.dirname(step.shot), { recursive: true });
      fs.writeFileSync(step.shot, Buffer.from(r.data, 'base64'));
      const [w, h] = pngSize(step.shot);
      if (w !== size[0] || h !== size[1]) {
        throw new Error(`${step.shot} is ${w}x${h}, not ${size[0]}x${size[1]}`);
      }
      console.log(`picture ${step.shot} ${w}x${h}`);
    }
  }
}

let code = 0;
try {
  await main();
} catch (e) {
  console.error(`shot failed: ${e.message}`);
  code = 1;
} finally {
  try { ws && ws.close(); } catch { /* closing anyway */ }
  chrome.kill();
  await sleep(500);
  try { fs.rmSync(profile, { recursive: true, force: true }); } catch { /* left for the machine */ }
}
process.exit(code);
