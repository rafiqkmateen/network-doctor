# Test results

Purpose: the result of each test case against its requirement, with what was seen. Never claims more than was run.
Owner: Rafiq. Version 0.1.0. Date: 2026-11-11. Machine: Windows 11 laptop, wired Ethernet (1 Gbps), Wi-Fi card present and disconnected.

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
| ND-11 speed, delay, loss | PASS | Live: 48 ms to the internet against 49 ms by hand; 129 to 149 Mbps down. Router delay reads 0 ms because it is under 1 ms. |
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
| ND-101 tests on three systems | PENDING | The workflow is written; it runs after the first push. |

Also found: Windows Application Control blocked an unsigned test program in a temporary folder. `netdoctor.exe` itself ran. On a laptop with strict app control the tool will need a code-signing certificate (change list).
