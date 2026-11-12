# Changelog

Purpose: every change to Network Doctor, newest first, in Keep a Changelog form.
Owner: Rafiq. Version 0.1.0.

## [Unreleased]

### Changed
- The speed test now opens six connections at once and counts bytes over four seconds after a one-second warm-up, instead of one half-second file on one connection. On a line the website measured at 918 down, the old test read 339; the new one reads about 780 to 800 (change list 12).

- When the speed server refuses a test ("too many requests"), the tool now says the speed was not measured and when to try again, instead of showing 0 Mbps. The JSON has `speed_measured` and, after a refusal, `speed_retry_after_seconds`. The tool also waits out the refusal between runs (`speed-hold.txt`), and the weekly check no longer saves a row without a measured speed (change list 14 and 15, ND-180 to ND-182).

- The speed test is lighter on the public server: four connections for three seconds (was six for four), and a speed measured less than ten minutes ago is reused, with a note of how old it is (`speed_age_seconds` in the JSON, `speed-last.json` in the results folder) (change list 15, ND-183 and ND-184).

### Added
- MIT License (`LICENSE`) and the notice for the one outside library, `rsc.io/qr` (`THIRD-PARTY-NOTICES.txt`) (matrix row 4).
- `netdoctor share`: finds this computer's address, serves the page for that network only, draws a code for a phone to scan, and says when a phone reaches the page (change list 9).
- `netdoctor setup`, the first-run helper: asks a few plain questions and starts by itself only for a person at a keyboard (change list 10).
- `netdoctor config alerts-file on|off`, and one tab-split line per alert in `alerts.log` (row 2 of the matrix).
- Linux and Mac readers for adapter, Wi-Fi, line use, programs, devices and settings. Linux also reads connection history; Mac does not yet (change list 3 and 13).
- Windows installer (`installer/netdoctor.iss`, Inno Setup): the on-demand check always installs; the weekly check and alerts are optional tick boxes; silent switches for IT; an uninstaller that undoes exactly what the install did and keeps saved results by default. Installed copies save in ProgramData. Tested on GitHub's clean Windows machine for all four choices (change list 11).
- `netdoctorw.exe`, a build with no console window, used by the weekly and alerts tasks.
- Install and removal section in the guide.

## [0.1.0] - 2026-11-11

### Added
- Project instructions, requirements, test cases and test results.
- Stage 1 of Network Doctor: full check (wired and Wi-Fi), cause finder, drops from the system log, link health, program use, device list, settings review.
- Command line with JSON output and exit codes, a local web page that can be opened from a phone on allowed addresses, weekly record to a spreadsheet file with a draft complaint (never sent), and watch mode with alerts.
- Release workflow with checksums and signed build records, and a test workflow for Windows, Mac and Linux (run after the first push).

### Fixed during live testing
- Sub-second connection flaps no longer bury real drops.
- This computer is no longer flagged as an unknown device; empty address entries are dropped.
- Power saving is reported as "could not read" when Windows will not say, not "off".
- Program ranking uses open connections and the line total, because per-program byte counts need administrator rights.
