package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// wiringChecks loads the whole page's scripts in Node with a small stand-in for the browser
// (document, fetch, timers) and presses its buttons, so the drawing code is tested too, not only
// the rules. Example data only.
const wiringChecks = `
const fs = require('fs'), vm = require('vm');
const html = fs.readFileSync(process.argv[2], 'utf8');
const scripts = [...html.matchAll(/<script(?: id="logic")?>([\s\S]*?)<\/script>/g)].map(m => m[1]);
let failed = 0;
function check(name, ok) { if (!ok) { failed++; console.log('FAIL: ' + name); } }

class El {
  constructor(tag) { this.tagName = tag; this.children = []; this.parent = null; this._text = ''; this.className = '';
    this.attrs = {}; this.style = {}; this.listeners = {}; this.hidden = false; this.disabled = false; this.value = ''; }
  get textContent() { return this._text + this.children.map(c => c.textContent).join(''); }
  set textContent(v) { this._text = String(v); this.children = []; }
  appendChild(c) { if (c.parent) c.remove(); c.parent = this; this.children.push(c); return c; }
  replaceChildren(...cs) { this.children = []; this._text = ''; cs.forEach(c => this.appendChild(c)); }
  remove() { if (this.parent) { this.parent.children = this.parent.children.filter(x => x !== this); this.parent = null; } }
  after(n) { const p = this.parent; if (n.parent) n.remove(); n.parent = p; p.children.splice(p.children.indexOf(this) + 1, 0, n); }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  addEventListener(t, f) { (this.listeners[t] = this.listeners[t] || []).push(f); }
  click() { (this.listeners.click || []).forEach(f => f({})); }
  get childElementCount() { return this.children.length; }
  all() { return [this].concat(...this.children.map(c => c.all())); }
  querySelector(sel) { const c = sel.slice(1); return this.all().slice(1).find(e => e.className.split(' ').includes(c)) || null; }
}

async function run(responses, alive) {
  const ids = {};
  ['when', 'go2', 'hero', 'herotext', 'out', 'go', 'state', 'settings'].forEach(id => ids[id] = new El('div'));
  const posts = [], timers = [], intervals = [];
  const document = { getElementById: id => ids[id], createElement: t => new El(t), addEventListener() {}, hidden: false };
  const fetch = async (path, opt) => {
    opt = opt || {};
    if (!opt.headers || opt.headers['X-NetDoctor'] !== '1') throw new Error('no page header on ' + path);
    if (path === '/api/alive') {
      if (alive && alive.length && !alive.shift()) throw new Error('gone');
      return {ok: true, status: 200};
    }
    if (opt.method === 'POST') { posts.push({path, body: JSON.parse(opt.body)}); return {ok: true, status: 200}; }
    const r = responses.shift();
    return {ok: r.status === 200, status: r.status, headers: {get: k => (r.headers || {})[k] || null}, json: async () => r.body};
  };
  const ctx = vm.createContext({document, fetch, console, Promise, Set, Math, Date, JSON, String, Error,
    setInterval: (f, ms) => intervals.push({f, ms}), setTimeout: (f, ms) => timers.push({f, ms})});
  vm.runInContext(scripts.join(';'), ctx);
  const settle = async () => { for (let i = 0; i < 20; i++) await new Promise(r => setImmediate(r)); };
  await settle();
  return {ids, posts, timers, intervals, settle, all: () => ids.out.all().concat(ids.settings.all())};
}

const dev = (n, extra) => Object.assign({name: '', maker: 'Example', ip: '192.168.1.' + n, mac: '02:00:00:00:00:' + n, known: false}, extra);
const report = () => ({
  time: new Date(2026, 2, 10, 16, 5).toISOString(), clock_24h: true, known_count: 0, warn_new_devices: true,
  finding: {cause: 'none', summary: 'Your connection looks healthy.'},
  reading: {kind: 'wired', link_mbps: 1000, speed_measured: true, down_mbps: 500, up_mbps: 50},
  speed_age_seconds: 130, drops: {count: 2, brief_blips: 0, drops: []}, notes: [], settings: [],
  devices: [dev(31, {name: 'tv'}), dev(32), dev(33)]});

(async () => {
  // A second page meets a running check, waits, then shows the result with working buttons.
  let p = await run([{status: 429}, {status: 200, headers: {'X-NetDoctor-Save': '1'}, body: report()}]);
  check('busy: says it is waiting', p.ids.state.textContent === 'A check is already running. Waiting for it...');
  check('busy: tries again in 10 seconds', p.timers.length === 1 && p.timers[0].ms === 10000);
  p.timers.shift().f();
  await p.settle();
  const text = p.ids.out.textContent, hero = p.ids.herotext.textContent;
  check('result shown after waiting', text.indexOf('Devices on your network') >= 0);
  check('healthy with drops says so in the box', hero.indexOf('Working now. It dropped 2 times in the last 7 days.') >= 0);
  check('the chart uses the same words', text.indexOf('Drops in the last 7 days') >= 0);
  check('reused speed labelled beside the tiles', text.indexOf('Earlier reading, from 2 minutes ago') >= 0);
  check('addresses on the rows', text.indexOf('192.168.1.32 / 02:00:00:00:00:32') >= 0);
  check('time in the computer\'s 24-hour format', p.ids.when.textContent.indexOf('16:05') >= 0);
  check('Check again at the top too', p.ids.go2.hidden === false);
  check('neutral wording first', text.indexOf('Tell Network Doctor which of these are yours') >= 0 && text.indexOf('Not marked yours') < 0);
  const btn = (txt) => p.all().filter(e => e.tagName === 'button' && e.textContent === txt);
  check('one This one is mine per device', btn('This one is mine').length === 3);

  // Pressing Ignore saves, marks the row, and keeps the others neutral.
  btn('Ignore')[1].click();
  await p.settle();
  check('Ignore posts the choice', p.posts.length === 1 && p.posts[0].path === '/api/known' && p.posts[0].body.devices[0].state === 'ignored');
  let t2 = p.ids.out.textContent;
  check('ignore alone keeps the others neutral', t2.indexOf('Not marked yours') < 0 && t2.indexOf('1 device ignored') >= 0);
  // Pressing This one is mine on a row shows Saved. beside it and turns tags on.
  btn('This one is mine')[0].click();
  await p.settle();
  t2 = p.ids.out.textContent;
  const savedRows = p.all().filter(e => e.className === 'saved');
  check('Saved. beside the row pressed', savedRows.length === 1 && savedRows[0].parent.textContent.indexOf('tv') >= 0);
  check('after a first mine the others are marked', t2.indexOf('Not marked yours') >= 0 && t2.indexOf('Yours') >= 0);

  // A page that cannot save has no buttons and says so.
  p = await run([{status: 200, body: report()}]);
  check('no buttons without the save mark', p.all().every(e => e.tagName !== 'button'));
  check('says this page cannot change choices', p.ids.settings.textContent.indexOf('This page cannot change your choices.') >= 0);
  // The heartbeat: two misses in a row are not enough to say it stopped; a success resets the
  // count; the third miss in a row does.
  p = await run([{status: 200, body: report()}], [false, false, true, false, false, false]);
  const beat = async () => { p.intervals[0].f(); await p.settle(); };
  const stopped = () => p.ids.state.textContent === 'Network Doctor has stopped. Start it again.';
  check('heartbeat every 5 seconds', p.intervals.length === 1 && p.intervals[0].ms === 5000);
  await beat(); check('one miss: still running', !stopped());
  await beat(); check('two misses: still running', !stopped());
  await beat(); check('a success resets the count', !stopped());
  await beat(); await beat(); check('two misses after the reset: still running', !stopped());
  await beat(); check('three misses in a row: stopped', stopped());
  process.exit(failed ? 1 : 0);
})().catch(e => { console.log('FAIL: ' + e.stack); process.exit(1); });
`

// ND-262, ND-264 and ND-265: the whole page, drawn with a stand-in browser in Node, waits for a
// running check, labels a reused speed, shows addresses and "Saved." on the row pressed, keeps the
// list neutral until a first mine, sends its header, and shows no buttons where saving would fail.
func TestPageWiringBehaves(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}
	dir := t.TempDir()
	pagePath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(pagePath, page, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wiring.js"), []byte(wiringChecks), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "wiring.js", pagePath)
	cmd.Dir = dir
	if outp, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("the page's wiring failed:\n%s", outp)
	}
}
