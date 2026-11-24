package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// logicChecks runs against the page's own rules (the script with id "logic") in Node. Each check
// throws when the behaviour is wrong. Example data only.
const logicChecks = `
const L = require('./logic.js');
let failed = 0;
function check(name, ok) { if (!ok) { failed++; console.log('FAIL: ' + name); } }
const dev = (mac, extra) => Object.assign({name: '', maker: '', ip: '192.168.1.' + mac.slice(-2), mac: mac, known: false}, extra);
const base = () => ({
  finding: {cause: 'none'}, drops: {count: 0}, warn_new_devices: true, known_count: 3,
  router_page: 'http' + '://192.168.1.1/',
  devices: [
    dev('02:00:00:00:00:31', {name: 'tv', known: true, state: 'mine', label: 'den tv'}),
    dev('02:00:00:00:00:32', {state: 'not-mine'}),
    dev('02:00:00:00:00:33', {name: 'printer', state: 'ignored'}),
    dev('02:00:00:00:00:34', {maker: 'unknown'}),
    dev('02:00:00:00:00:35', {maker: 'private address (device hides its maker)'}),
  ]});

// Choices made, warnings on.
let v = L.deviceView(base(), true);
check('ignored devices are not in the list', v.rows.every(r => r.device.state !== 'ignored'));
check('ignored devices are listed apart', v.ignored.length === 1 && v.ignored[0].mac === '02:00:00:00:00:33');
check('not mine is first', v.rows[0].device.state === 'not-mine' && v.rows[0].tag === 'Not mine');
check('stranger count leaves out ignored and not mine', v.strangers === 2 && v.sub === '5 devices found, 2 not marked yours.');
check('mine shows Yours with its saved name', v.rows.some(r => r.name === 'den tv' && r.tag === 'Yours'));
check('undecided shows Not marked yours', v.rows.filter(r => r.tag === 'Not marked yours').length === 2);
check('the not-mine box shows with warnings on', v.notMineBox && v.notMine === 1 && v.routerPage !== '');
check('no Tell sentence once choices exist', !v.tell && !v.offNote);
check('All of these are mine takes only the undecided', v.allMine.length === 2 && v.allMine.every(m => m.state === 'mine'));
check('buttons only when the page can save', v.rows.every(r => r.actions));
check('maker unknown reads Maker not known', v.rows.some(r => r.maker === 'Maker not known'));
check('hidden maker in plain words', v.rows.some(r => r.maker === 'Maker hidden by the device'));

// Warnings off: no stranger warnings at all.
const off = base(); off.warn_new_devices = false;
v = L.deviceView(off, true);
check('switch off: no Not marked yours tags', v.rows.every(r => r.tag !== 'Not marked yours'));
check('switch off: no stranger count', v.sub === '5 devices found.');
check('switch off: no not-mine box, and a note says it is off', !v.notMineBox && v.offNote);

// Nothing chosen yet: neutral wording, no tags.
const fresh = base(); fresh.known_count = 0;
fresh.devices.forEach(d => { d.state = ''; d.known = false; delete d.label; });
v = L.deviceView(fresh, true);
check('nothing chosen: Tell sentence', v.tell && v.sub === '5 devices found.');
check('nothing chosen: no tags', v.rows.every(r => r.tag === ''));

// A page that cannot save shows no buttons.
v = L.deviceView(base(), false);
check('cannot save: no buttons', v.rows.every(r => !r.actions) && v.allMine.length === 0);

// Saved choices show at once.
let r = L.applyMarks(base(), [{mac: '02-00-00-00-00-34', name: '  <b>kid</b> ', state: 'mine'}, {mac: '02:00:00:00:00:33', name: '', state: 'undecided'}]);
const d34 = r.devices.find(d => d.mac.endsWith('34')), d33 = r.devices.find(d => d.mac.endsWith('33'));
check('mark mine: known with the name as typed', d34.known && d34.state === 'mine' && d34.label === '<b>kid</b>');
check('bring back: undecided again', d33.state === '' && !d33.known);
check('count follows the choices: mine counts, bringing back an ignored one does not', r.known_count === 4);
check('switch applies', L.applyWarn(base(), false).warn_new_devices === false);

// Times follow clock_24h; without it the browser decides.
const t = new Date(2026, 2, 10, 16, 5);
check('24-hour clock', L.clockText(t, true, 'en-US').indexOf('16:05') >= 0);
check('12-hour clock', /4:05/.test(L.clockText(t, false, 'en-US')) && /PM/.test(L.clockText(t, false, 'en-US')));
check('12-hour even where the language uses 24', /PM|pm/.test(L.clockText(t, false, 'en-GB')));
check('24-hour even where the language uses 12', L.clockText(t, true, 'en-US').indexOf('PM') < 0);
check('no field: no clock forced', !('hour12' in L.timeOpts(undefined)));
check('pattern hour in 24-hour', L.patternSentence({pattern_hour: 4}, true, 'en-US').indexOf('04:00') >= 0);
check('pattern hour in 12-hour', /4:00.AM/.test(L.patternSentence({pattern_hour: 4}, false, 'en-US')));
check('pattern falls back to the sentence', L.patternSentence({pattern: 'x'}, true) === 'x');
const notes = L.notesFor({notes: ['Too soon to test again. old', 'other'], speed_retry_at: t.toISOString(), speed_age_seconds: 180, speed_wait_seconds: 420, clock_24h: true}, 'en-US');
check('too-soon note rebuilt once', notes.length === 2 && notes[0].indexOf('(at 16:05)') > 0 && notes[0].indexOf('3 minutes ago') > 0 && notes[0].indexOf('in about 7 minutes') > 0);
check('healthy with drops says so', L.healthyDropsLine({finding: {cause: 'none'}, drops: {count: 3}}) === 'Working now. It dropped 3 times in the last 7 days.');
check('healthy with one drop', L.healthyDropsLine({finding: {cause: 'none'}, drops: {count: 1}}) === 'Working now. It dropped once in the last 7 days.');
check('no line when not healthy or no drops', L.healthyDropsLine({finding: {cause: 'router'}, drops: {count: 3}}) === '' && L.healthyDropsLine({finding: {cause: 'none'}, drops: {count: 0}}) === '');
// Addresses on each row.
v = L.deviceView(base(), true);
check('each row shows its address', v.rows.every(row => row.address === row.device.ip + ' / ' + row.device.mac));
// Ignoring alone keeps the list neutral; a first Mine switches the tags on.
let n = base(); n.known_count = 0;
n.devices.forEach(d => { d.state = ''; d.known = false; delete d.label; });
n = L.applyMarks(n, [{mac: '02:00:00:00:00:33', name: '', state: 'ignored'}]);
v = L.deviceView(n, true);
check('ignore alone: still neutral, no tags', n.known_count === 0 && v.tell && v.rows.every(row => row.tag === ''));
check('ignore alone: the ignored device is set apart', v.ignored.length === 1 && v.rows.length === 4);
n = L.applyMarks(n, [{mac: '02:00:00:00:00:31', name: '', state: 'mine'}]);
v = L.deviceView(n, true);
check('first mine: tags appear', n.known_count === 1 && !v.tell && v.rows.some(row => row.tag === 'Not marked yours'));
// A reused speed says so beside the numbers.
check('reused speed label', L.speedAgeLabel({reading: {speed_measured: true}, speed_age_seconds: 130}) === 'Earlier reading, from 2 minutes ago');
check('no label for a new speed', L.speedAgeLabel({reading: {speed_measured: true}}) === '' && L.speedAgeLabel({reading: {speed_measured: false}, speed_age_seconds: 60}) === '');
// A check already running is waited for: every 10 seconds, 12 times at most.
check('busy: waits and retries', L.busyStep(0).retry && L.busyStep(0).wait === 10000 && L.busyStep(0).text === 'A check is already running. Waiting for it...');
check('busy: gives up after 12', L.busyStep(11).retry && !L.busyStep(12).retry);
// Drops not on the 7 day bars are counted in plain words, so bars plus the note equal the headline.
check('one earlier drop', L.offChartNote(10, 9) === '1 earlier drop is not on this chart.');
check('two earlier drops', L.offChartNote(5, 3) === '2 earlier drops are not on this chart.');
check('no note when the bars hold them all', L.offChartNote(4, 4) === '' && L.offChartNote(0, 0) === '' && L.offChartNote(2, 3) === '');
process.exit(failed ? 1 : 0);
`

// ND-262, ND-264 and ND-265: the page's own rules behave as required: ignored devices are hidden and
// not counted, not-mine stays on top, the switch removes warnings, buttons show only when saving
// works, and times follow clock_24h. Runs the page's logic script in Node; skipped without Node.
func TestPageLogicBehaves(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}
	s := string(page)
	const open = `<script id="logic">`
	i := strings.Index(s, open)
	if i < 0 {
		t.Fatal(`the page has no <script id="logic"> block`)
	}
	j := strings.Index(s[i:], "</script>")
	if j < 0 {
		t.Fatal("the logic block is not closed")
	}
	logic := s[i+len(open) : i+j]
	if strings.Contains(logic, "document.") || strings.Contains(logic, "window.") {
		t.Error("the logic block must not touch the page, so it can be tested on its own")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "logic.js"), []byte(logic), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checks.js"), []byte(logicChecks), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "checks.js")
	cmd.Dir = dir
	if outp, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("the page's logic failed:\n%s", outp)
	}
}
