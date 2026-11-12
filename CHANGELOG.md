# Changelog

Purpose: every change to Network Doctor, newest first, in Keep a Changelog form.
Owner: Rafiq. Version 0.2.0.

## [Unreleased]

### Changed
- The front page and the guide no longer say the tool "never changes a setting". They say checks only look, and describe the optional `fix` commands, the private phone page (https) and its one-time warning.

## [0.2.0] - 2026-11-12

### Added
- Ookla's own Speedtest tool is used for the speed number when the person has put it where we look (the `NETDOCTOR_OOKLA` setting, an `ookla` folder beside the program, or the path); otherwise the Cloudflare test runs as before. The answer names the tool (`speed_tool`). Network Doctor never ships Ookla's tool and never accepts Ookla's license for anyone (change list 20, ND-200 to ND-203).
- The installer can offer Ookla's tool on its own page: unchecked, never in a silent install, with the person's own license tick, fetched only from Ookla and checked against a fixed hash (ND-210).
- A choices file, `choices.json`, written beside the program by the installer: which parts the person chose. Running the installer again repairs the machine to match: a part not chosen has its task removed; the uninstaller removes the file (change list 19, ND-207 to ND-209).

### Changed
- A speed is reused for 2 minutes when Ookla's tool measured it and for 10 minutes when Cloudflare measured it. A check inside the hold measures nothing and says "Too soon to test again", how old the reading is, and when to come back (`speed_wait_seconds`) (change list 18, ND-204 to ND-206).
- The repeating alerts helper uses only the light Cloudflare test, never Ookla's, which moves gigabytes a test.
- The program version is 0.2.0.

## [0.1.0] - 2026-11-12

### Changed
- The speed test now opens six connections at once and counts bytes over four seconds after a one-second warm-up, instead of one half-second file on one connection. On a line the website measured at 918 down, the old test read 339; the new one reads about 780 to 800 (change list 12).

- When the speed server refuses a test ("too many requests"), the tool now says the speed was not measured and when to try again, instead of showing 0 Mbps. The JSON has `speed_measured` and, after a refusal, `speed_retry_after_seconds`. The tool also waits out the refusal between runs (`speed-hold.txt`), and the weekly check no longer saves a row without a measured speed (change list 14 and 15, ND-180 to ND-182).

- The speed test is lighter on the public server: four connections for three seconds (was six for four), and a speed measured less than ten minutes ago is reused, with a note of how old it is (`speed_age_seconds` in the JSON, `speed-last.json` in the results folder) (change list 15, ND-183 and ND-184).

### Fixed
- The power saving fix no longer says "this adapter does not report a power saving setting" when a driver will not let Windows read it. It says it cannot read the setting and gives the Device Manager steps (found live: the Realtek wired adapter on the maintainer's laptop has the setting on, but its driver refuses the read).

### Added
- Stage 2: `netdoctor fix list | apply NAME [--yes] | undo NAME | enable-here`. Three fixes (clear the name cache, turn off a proxy, turn off adapter power saving), each only after a yes to that one fix, with the setting shown before and after, the old value saved, one-press undo and a log (`fix-log.txt`). Power saving only says what to change by hand until a test shows it never drops the link. Off on an unattended server (`NETDOCTOR_UNATTENDED`) until `fix enable-here` (change list 5, ND-90 to ND-96).
- HTTPS for the phone page: `share` is private by default (`--http` for plain) with a certificate made on this computer, and `serve --https` (change list 16, ND-190 to ND-192).
- MIT License (`LICENSE`) and the notice for the one outside library, `rsc.io/qr` (`THIRD-PARTY-NOTICES.txt`) (matrix row 4).
- `netdoctor share`: finds this computer's address, serves the page for that network only, draws a code for a phone to scan, and says when a phone reaches the page (change list 9).
- `netdoctor setup`, the first-run helper: asks a few plain questions and starts by itself only for a person at a keyboard (change list 10).
- `netdoctor config alerts-file on|off`, and one tab-split line per alert in `alerts.log` (row 2 of the matrix).
- Linux and Mac readers for adapter, Wi-Fi, line use, programs, devices and settings. Linux also reads connection history; Mac does not yet (change list 3 and 13).
- Windows installer (`installer/netdoctor.iss`, Inno Setup): the on-demand check always installs; the weekly check and alerts are optional tick boxes; silent switches for IT; an uninstaller that undoes exactly what the install did and keeps saved results by default. Installed copies save in ProgramData. Tested on GitHub's clean Windows machine for all four choices (change list 11).
- `netdoctorw.exe`, a build with no console window, used by the weekly and alerts tasks.
- Install and removal section in the guide.

### Also in this release (first built 2026-11-11)
- Project instructions, requirements, test cases and test results.
- Stage 1 of Network Doctor: full check (wired and Wi-Fi), cause finder, drops from the system log, link health, program use, device list, settings review.
- Command line with JSON output and exit codes, a local web page that can be opened from a phone on allowed addresses, weekly record to a spreadsheet file with a draft complaint (never sent), and watch mode with alerts.
- Release workflow with checksums and signed build records, and a test workflow for Windows, Mac and Linux (run after the first push).

### Fixed during live testing
- Sub-second connection flaps no longer bury real drops.
- This computer is no longer flagged as an unknown device; empty address entries are dropped.
- Power saving is reported as "could not read" when Windows will not say, not "off".
- Program ranking uses open connections and the line total, because per-program byte counts need administrator rights.
