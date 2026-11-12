# Test cases

Purpose: one test per requirement, written before the build. Each says what is done and what counts as a pass.
Owner: Rafiq. Version 0.6.0 (0.6.0: ND-200 to ND-211 added; 0.5.0: ND-183 and ND-184 added; 0.4.0: ND-180 to ND-182 added; 0.3.0: ND-130 to ND-172 added; 0.2.1: speed case added; 0.2.0: ND-110 to ND-120 added; 0.1.0: draft). Date: 2026-11-11.

Method: A = automated Go test. M = measured on this laptop, result shown to Rafiq. G = checked by the GitHub test run on Windows, Mac and Linux.
Status of every test today: not run (nothing built yet).

| Req | Method | Test | Pass when |
|---|---|---|---|
| ND-01 | A | Snapshot network settings, run every stage 1 command, snapshot again. | The two snapshots are identical. |
| ND-02 | M, G | Build each target; copy the single file to a clean folder and run `--version`. | Runs with no install; zip holds the file and the one-page guide. |
| ND-03 | A | Block internet, run all checks except speed and internet. | They finish and report; the two online checks say "offline" and do not crash. |
| ND-04 | A | Watch outgoing connections during a full run. | None except the speed test and internet checks. |
| ND-05 | A | Run a readability score on every message and explanation. | Grade 8 or lower. |
| ND-10 | A, M | Run with the cable plugged and Wi-Fi off, then Wi-Fi only. | Reports "wired" then "Wi-Fi" correctly. |
| ND-11 | M | Run on this laptop; compare to a known router ping. | Router and internet speed, delay and loss reported within 10 percent of a manual check. |
| ND-11 (speed) | G | Run the speed test against a pretend server; then run it live beside a speed website. | At least four connections at once, a short run; live result close to the website's, allowing for a different server. |
| ND-12 | A | Feed recorded readings for four fault cases (computer, cable or signal, router, provider). | Each case names the right cause and a first thing to check. |
| ND-20 | M | Start a large download; run the check. | That program ranks first with upload and download shown. |
| ND-21 | A | Run with a download active. | Advice names it; the download is still running afterward. |
| ND-30 | A | Feed a recorded week of connection history with 3 known drops. | Reports 3 drops with correct times and lengths. |
| ND-31 | A | Feed history with drops at the same hour each night. | Reports the time-of-day pattern. |
| ND-40 | M | Run wired. | Link speed, duplex and error counts match the adapter's own page. |
| ND-41 | M | Run on Wi-Fi. | Signal, band and channel match the system's own report; neighbors on the same channel listed. |
| ND-50 | M | Run on this network. | Every device on the router's list appears with name, maker and both addresses; unmarked ones flagged. |
| ND-51 | A | Search the code, logs, files and memory dump for any entered router password after a run. | Not found anywhere. |
| ND-60 | M | Run on this laptop. | Each of the six areas is reported with a plain explanation of the effect of changing it. |
| ND-70 | M | Run one command or press one button. | A full check completes and shows results. |
| ND-71 | A | Turn on weekly check; advance the clock to Monday 0900. | One row (speed, delay, date, time, plan) is added to the spreadsheet. |
| ND-72 | A | Feed three weeks under 50 percent, then three weeks with one above. | A draft appears after three; none when broken by a good week; nothing is ever sent. |
| ND-73 | A | Simulate a drop, a slow stretch and a new device. | One notification each. |
| ND-74 | A | Switch each alert off; set quiet hours; simulate one long outage. | Switched-off alerts silent; quiet hours honored; one alert for the outage. |
| ND-75 | A | Run with default settings. | No alert leaves the machine. |
| ND-80 | A | Run command-line mode with no screen. | Valid JSON out; exit code 0 for healthy, nonzero and documented for each problem class. |
| ND-81 | A, M | Open the web page from this laptop and from a phone on the allowed network; try from a blocked address. | Works for allowed, refused for blocked. |
| ND-90 | A | Request a fix without approval, then with approval for one fix among several. | Nothing changes without approval; only the approved fix changes. |
| ND-91 | A | Approve a fix. | Before and after values were shown first. |
| ND-92 | A | Apply a fix, then undo. | Setting returns to the original value with one action. |
| ND-93 | A | Apply and undo a fix. | Log has an entry for each with time, setting, old and new value. |
| ND-94 | M | Run stage 1 as a normal user, then approve a fix. | Stage 1 never asks for administrator rights; the approval does, once. |
| ND-95 | A | Try to apply a driver change, a security change and a change that cuts the active link. | All three refused with an explanation. |
| ND-96 | A | Run in server mode with defaults, then switch apply on for that machine. | Apply refused by default; works only after the per-machine switch. |
| ND-100 | G | Make a release. | Checksum file and signature present and verify. |
| ND-101 | G | Push a change. | Test runs complete on Windows, Mac and Linux. |
| ND-110 | G | On GitHub's clean Windows machine, install with no other setup. | Install finishes and `netdoctor check` runs. |
| ND-111 | G | Run the silent install with no parts chosen, and look at the window's list in a script check. | The on-demand check is installed and cannot be cleared in the component list. |
| ND-112 | A | Read the installer script. | Each optional part has a plain sentence and its own tick box, off until chosen. |
| ND-113 | G | Install each of the four choices. | Files and tasks match the choice exactly, with nothing extra. |
| ND-114 | G | Install silently with each switch combination. | Same result as the window, and no window opens. |
| ND-115 | G | Install, then read the folders and the Apps and features list. | Program in Program Files, entry present; the install ran with administrator rights. |
| ND-116 | A | Run the storage code once as installed and once unzipped. | Installed uses ProgramData; unzipped uses the person's own folder. |
| ND-117 | G | Install each choice and list the tasks. | A weekly task only with scheduled; a log-on task only with proactive. |
| ND-118 | G | Uninstall after each choice, keeping and then not keeping results. | Program, tasks and entry gone; results kept by default and gone when asked. |
| ND-119 | A | Scan the installer script and the setup code. | No upload, mail or web call, except Ookla's own addresses over https (ND-210). |
| ND-120 | G | Run the whole install and uninstall set in the GitHub run. | All four choices pass on a fresh machine. |
| ND-130 | A | Start `share` with a pretend adapter and a pretend VPN. | The address printed is the real adapter's; the page answers on it. |
| ND-131 | A | Make the code for a known address. | A square code with a clear border; reading it back gives the same address. |
| ND-132 | A | Start `share`; send a request from a pretend other device, then send none. | The first request is announced; with none, the advice appears after the wait. |
| ND-133 | A | Ask the page from an address outside the network; scan the code for firewall or setting commands. | Refused from outside; no change command anywhere. |
| ND-134 | A | Start `share` with no usable address. | A plain message and no page. |
| ND-140 | A | Run `setup` with typed answers. | Plan, name and chosen known devices saved; each offer asked once. |
| ND-141 | A | Run `check --json`, `weekly`, plain `netdoctor` with and without a keyboard and with and without a settings file. | A question is asked only for a keyboard, no command and no settings file. |
| ND-142 | A | Read every helper sentence; press Enter on each question. | Each sentence grade 8 or lower; Enter means no. |
| ND-143 | A | Answer no to everything. | Nothing saved except what was typed (plan, name). |
| ND-150 | A | Run `config alerts-file on` then `off` with other places set. | The file place comes and goes; the others stay. |
| ND-151 | A | Send an alert with the file on and with it off. | One line of time, kind and message when on; no file when off. |
| ND-152 | A | Send an alert with the file on. | The file sits beside `weekly.csv`. |
| ND-160 | A | Read sample text from the Linux commands and files. | Adapter, Wi-Fi, line use, programs, history, devices and settings come out right. |
| ND-161 | A | Feed sample text with the admin-only parts missing. | The tool says it could not read them. |
| ND-162 | A | Run the sample-text tests on Windows, Mac and Linux. | They pass on all three. |
| ND-163 | A | Scan every Linux and Mac command. | Only read verbs. |
| ND-164 | G | On GitHub's Mac and Linux machines run `check --json --quick`. | The adapter kind is wired or Wi-Fi and its fields are filled. |
| ND-170 | A | Read sample text from the Mac commands. | Same as ND-160. |
| ND-171 | A | Feed sample text with admin-only parts missing. | The tool says it could not read them. |
| ND-172 | A | Run the Mac sample-text tests on all three systems. | They pass on all three. |
| ND-180 | A | A pretend speed server answers "too many requests" with a wait; a pretend machine refuses the speed test. | The wait is read; the check says not measured, says to try again in the right minutes, and carries the wait in its JSON. A finished test says measured. |
| ND-181 | A | Run two checks in a row after a refusal; try a finished, missing and damaged hold file. | The second check never calls the speed test; a finished, missing or damaged hold means none. |
| ND-182 | A | Run the weekly check three times with a refused speed test. | No row is saved and no complaint is drafted. |
| ND-183 | A | Read the default speed plan. | Four connections or fewer, three seconds or less. |
| ND-184 | A | Check with a speed from three minutes ago, then with one from eleven minutes ago; run two checks in a row. | The first reuses it with a note of its age; the second measures again; two checks in a row measure once. A refused test saves nothing. |
| ND-184 | L | On the real server, run two checks in a row, then compare the speed with the earlier 857 down. | One real test only; the speed is close to the earlier reading. |
| ND-190 | A | Run `share` with a pretend server and a certificate folder; run it again with `--http`; ask a real TLS server for its health page. | https address printed and served with a certificate on the adapter address and its own network only; `--http` stays plain; the real server answers over TLS 1.2 or newer and keeps the listen guard. |
| ND-191 | A | Make a certificate; ask for it again; ask for a new address; ask 350 days later; damage the file. | Covers the address, lasts about a year, key private; reused while good; replaced for a new address, near the end, or when damaged. |
| ND-192 | A | Read the `share` output with https and with `--http`; make the certificate fail. | The warning and what to press are said with https only; a failed certificate says so and uses the plain address. |
| ND-190 | L | Scan the code on a phone, press Advanced then Continue. | The page opens over https and shows a check. |
| ND-200 | A | Parse a result line with 110000000 and 100000000 bytes a second among other lines; parse empty, junk and zero results. | 880 and 800 megabits; the others are errors. |
| ND-201 | A | Name a tool by setting, by a folder beside the program, by a missing setting path, and with nothing anywhere. | Found by setting, then folder; a setting that points nowhere finds nothing; nothing means none. |
| ND-202 | A | Measure with a working fake Ookla, a failing one, and none, next to a pretend Cloudflare server. | Ookla's numbers and name; Cloudflare's numbers and name when it fails or is missing. |
| ND-203 | A | Read the arguments Ookla's tool is run with. | They never carry a license switch. |
| ND-204 | A | Check with an Ookla speed 1 and 3 minutes old, a Cloudflare speed 3 and 11 minutes old, and one with no tool name 5 minutes old. | Held, free, held, free, held. |
| ND-205 | A | Check inside the hold. | Nothing measured; the note has too soon, the age, come back and a time; the wait is about a minute. |
| ND-206 | A | Save a sample with a tool name and read it back. | The name survives. |
| ND-207 | A | Read the exact text the installer writes; read a missing and a damaged file. | The four choices read back; the others say nothing. |
| ND-208 | A | Turn the weekly choice off in a folder with and without a choices file. | Only the weekly choice changes, and nothing is written without a file. |
| ND-209 | A | Read the installer script. | It writes the file from the selected parts, deletes the task of a part not chosen, and the uninstaller removes the file and the `ookla` folder. |
| ND-210 | A | Read the installer script. | Unchecked by default, skipped when silent, a separate license tick, Ookla's addresses only, a fixed SHA-256, the license switch once and only after the tick. |
| ND-209 | L | On a clean Windows machine, install each of the four choices, run the installer again with a different choice, then uninstall. | The file matches each choice, the tasks match it, repair removes a part not chosen, and uninstall removes the file. |
| ND-210 | L | On a laptop, tick the Ookla boxes in the window and finish. | The tool is fetched, unpacked beside the program, runs, and `choices.json` says `ookla` is true. |
| ND-211 | A | Run `netdoctor version` and look at the installer file name. | 0.2.0 in both. |
