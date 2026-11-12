# Test results

Purpose: the result of each test case against its requirement, with what was seen. Never claims more than was run.
Owner: Rafiq. Version 0.1.2 (0.1.2: ND-11 speed test made multi-connection; 0.1.1: ND-112 seen on Rafiq's laptop; 0.1.0: first results). Date: 2026-11-11. Machine: Windows 11 laptop, wired Ethernet (1 Gbps), Wi-Fi card present and disconnected.

Run: `go test ./...` passes on this laptop (core, collect, app, store, web). Builds for Windows, Linux (x64, ARM) and Mac (Intel, Apple) all compile.

Key: PASS = tested and met. PARTIAL = met in part; the gap is named. NOT BUILT = later stage. PENDING = needs the first push to GitHub.

| Req | Result | What was seen |
|---|---|---|
| ND-01 read-only | PASS | A test rejects any command with a changing verb in every script. Snapshot of adapters, advanced properties, DNS, addresses, routes, proxy, scheduled tasks and services taken before and after two full checks: identical. |
| ND-02 single file, zip | PASS (Windows) | `netdoctor.exe` (11 MB) unzipped into an empty folder ran `version`. Zips for all five targets built with the guide; the Mac and Linux files compile but were not run. |
| ND-03 offline | PASS (test) | With the internet down in a pretend machine, the two online checks say "offline" and the local checks still run. Not tried by pulling the real cable. |
| ND-04 nothing leaves | PASS (scan) | A test fails if the code can send mail or post data outside the speed test file. Real connections were not logged. |
| ND-05 plain English | PASS | Every sentence the tool writes scores grade 8 or lower. The test caught one sentence at grade 10 and it was reworded. |
| ND-10 wired or Wi-Fi | PARTIAL | Wired detected live. Wi-Fi detection and the Wi-Fi report are tested on sample text only, because the Wi-Fi card was disconnected. |
| ND-11 speed, delay, loss | PASS | Live: 48 ms to the internet against 49 ms by hand. 2026-11-11 (later): Rafiq saw the old test read 339 down and 111 up where a speed website read 918 and 641. The test now uses six connections for four seconds. Tests (a pretend server) show at least four run at once and the run stays short. Live after the change: 800 and 776 down, 812 and 917 up, delay 34 to 37 ms. The website uses a nearer server, so the numbers will not match exactly. Earlier, with the old test: 129 to 149 Mbps down. Router delay reads 0 ms because it is under 1 ms. |
| ND-12 cause | PASS | Four fault cases and a healthy case name the right cause and first step. |
| ND-20 programs ranked | PARTIAL | Windows gives per-program bytes only to administrators. The first method (program disk and network counters) read 27 KB a second during a 21 MB a second download, so it was dropped. Now: the line total (live, saw 3.9 MB a second during the tool's own upload) and programs ranked by open connections (live: chrome 12, claude 10, python 6). Exact per-program numbers are on the change list. |
| ND-21 pause advice | PASS | Advice names programs; nothing is stopped. |
| ND-30 drops | PASS | Three known drops counted with right times and lengths. Live: 6 real drops and 23 blips in 7 days. Blips under 5 seconds are counted apart (found live: the log holds many sub-second flaps). |
| ND-31 patterns | PASS | Same-hour pattern found in test and live (around 2000). |
| ND-40 wired link | PASS | Live: 1000 Mbps, full duplex, 0 errors, matching the adapter's own page. |
| ND-41 Wi-Fi link | PARTIAL | Parsing of the system's Wi-Fi report is tested on sample text; not run on a live Wi-Fi link. |
| ND-50 devices | PASS | Live: 15 entries found; empty entries removed; this computer no longer flagged. Not compared line by line with the router's own list. |
| ND-51 no password | PASS | Scan finds none in code. The tool never asks for one. |
| ND-60 settings | PASS | Live: all six areas reported. Found live: Windows would not report power saving for the wired adapter, and the tool had said "off". It now says it could not read it, and uses the adapter's own energy options when it can (this adapter has three on). |
| ND-70 on demand | PASS | `netdoctor check` gives a full report in about 33 seconds. |
| ND-71 weekly row | PASS | Live: one row written to `weekly.csv`. `schedule status` ran; `schedule enable` was not run on this laptop. |
| ND-72 complaint draft | PASS | Three low weeks draft, a good week breaks the run, and nothing is ever sent. |
| ND-73 alerts | PASS | One alert each for a drop, a slow stretch and a new device in test. Live: new-device alerts appeared during `watch`. |
| ND-74 switches, quiet, once | PASS | Tested. |
| ND-75 default sink | PASS | The default is the console only. |
| ND-80 command line | PASS | Valid JSON and the documented exit codes, in test and live. |
| ND-81 web page | PARTIAL | Page ran in the built-in browser on this laptop and gave the answer. Blocked and allowed addresses are tested. Not tried from a phone. |
| ND-90 to ND-96 | NOT BUILT | Stage 2 comes after stage 1 is accepted. |
| ND-100 checksums, signature | PARTIAL | Checksum file produced. The signature step is written into the release workflow and runs on the first release. |
| ND-101 tests on three systems | PASS | 2026-11-11 2338: the automated tests, vet and build passed on GitHub's Windows, Mac and Linux machines. Two earlier runs failed in the format check itself (a shell difference on Mac, line endings on Windows), not in the tool; both were fixed. These runs use the pretend-machine tests; the Mac and Linux readers of adapter, log, devices and settings are not built yet. |
| ND-110 | PASS | 2026-11-11: on GitHub's clean Windows machine, `setup.exe` installed with nothing else set up and `netdoctor version` ran (Network Doctor 0.1.0). |
| ND-111 | PASS | A script check finds the check-on-demand part marked fixed. The silent install with no parts chosen installed it and nothing else. |
| ND-112 | PASS | A script check finds each optional part with its own sentence and off by default. 2026-11-11: Rafiq opened the setup window on his laptop, ticked the boxes and installed it. Both tasks (weekly check and alerts) then existed on his laptop. |
| ND-113 | PASS | All four choices (neither, scheduled, proactive, both) installed exactly the right files and tasks. |
| ND-114 | PASS | The four runs used the silent switches with `/COMPONENTS`; no window opened. |
| ND-115 | PASS | Program in Program Files, entry in Apps and features, uninstaller in the program folder, install ran with administrator rights. |
| ND-116 | PASS | Installed copy saved `known-devices.txt` in ProgramData. A test shows the unzipped copy keeps its per-person folder. |
| ND-117 | PASS | The weekly task existed only with scheduled; the log-on task only with proactive. |
| ND-118 | PASS | Uninstall removed the program, both tasks and the Apps and features entry. Results were kept by default and deleted with the delete switch. Found on the way: the question box made a silent uninstall hang; a silent uninstall now keeps results without asking. |
| ND-119 | PASS | A test scans the script for web, mail and download words and finds none. |
| ND-120 | PASS | All four choices, install then uninstall, passed on GitHub's clean Windows machine on 2026-11-11 (run 43131744806). The Mac and Linux readers are still not built. |

Also found: Windows Application Control blocked an unsigned test program in a temporary folder. `netdoctor.exe` itself ran. On a laptop with strict app control the tool will need a code-signing certificate (change list).
