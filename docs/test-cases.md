# Test cases

Purpose: one test per requirement, written before the build. Each says what is done and what counts as a pass.
Owner: Rafiq. Version 0.1.0 (draft). Date: 2026-11-11.

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
