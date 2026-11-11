# Changelog

Purpose: every change to Network Doctor, newest first, in Keep a Changelog form.
Owner: Rafiq. Version 0.1.0.

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
