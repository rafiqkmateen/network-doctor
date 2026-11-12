# Handover

Purpose: the one job for the next session, and nothing else.
Owner: Rafiq. Version 0.3.1 (0.3.1: new setup installed 2026-11-12, blocked by Windows Application Control, matrix row 1 is the fault; 0.3.0: wrap of 2026-11-12, speed test fixed, share, setup helper, alerts file, Linux and Mac readers built and passing on GitHub; 0.2.1: code signing made its own row and rows put in dependency order; 0.2.0: wrap of 2026-11-11, pruned; installer seen and installed on his laptop, Khadijah now uses the installed copy, Khadijah-side rows moved to her Handover; 0.1.x: installer and stage 1). Date: 2026-11-11. Finished work is in `CHANGELOG.md`, not here. Keep this page under 1,500 words; prune it at every wrap.

## Do this first

1. Read this page, `CLAUDE.md`, `docs\change-list.md` and `docs\test-results.md`.
2. Show the matrix below once, then wait for his word "go" on a row.

## Hard rules

- Plain words, no codes; imperial units; military time; short, yes or no first. The full set is in memory.
- Only the word "go" starts work. "Agreed", an answer or a button pick is not a go. A question from him gets an explanation and nothing else: no edits, no matrix tacked on.
- Do the jobs I can do myself (uninstall, find a file, run a check). Ask him only for what is his: a press on a Windows box, a choice, a go.
- Test first, then build. A new idea goes on `docs\change-list.md`. Commit finished work at once, local only. Push only to the private repo on his word; make it public only on his word.
- Never work around a Windows security block. Finished files for him go in `C:\Users\rafiq\Downloads\Claude finished work\Network Doctor`.
- The demo video is a row on Khadijah's matrix, not here.
- Network Doctor stands on its own: no part may need Khadijah or Network Mapper. His three modes: reactive (always on), scheduled and proactive (each offered, explained in plain words, one press is the yes).
- One list: this page holds only Network Doctor's own rows. The Khadijah card rows (top-strip readout, verdicts, weekly history card, pictures) live on her Handover.

## The matrix

| # | What | Industry standard | State |
|---|------|-------------------|-------|
| 1 | FAULT: Windows Application Control blocks the new installed `netdoctor.exe` (not signed, so Windows does not trust it). Khadijah's card cannot run it until this is fixed. The fix is code signing: a paid certificate that proves the file comes from you. Or you allow the file yourself in Windows Security. I will not work around the block. | Sign what you ship. | Waits for your answer. Row 2 waits on it. |
| 2 | Scan the phone code: run `netdoctor share` and point your phone at the code. The new setup is installed; the page and code are tested but no real phone has opened the page yet. | Test on the real device. | Waits for row 1, then your press. |
| 3 | Mac connection history (drops over seven days). The Mac log is slow and changes between versions, so the Mac reader says "not available" for drops today. Build it or leave it. | Test on the real system. | Waits for his answer. |
| 4 | Pick a license: the rule that says what other people may do with the code. | Public code needs a license. | Waits for his answer. Comes before going public. |
| 5 | Make the repository public: a no-reply email, a front page, a first release. | Public code needs no personal data. | Waits for his "go", after rows 1 and 4. |
| 6 | Stage 2: let the tool fix a problem itself, only with his yes on each fix, a before and after view, one-press undo and a change log. | Least privilege, undo, audit log. | After stage 1 is accepted. |

## State (2026-11-11)

- Stage 1 is built (Go 1.27.4). `go test ./...` passes; GitHub passes on Windows, Mac and Linux, and a fourth job tests the installer (run 43146554958, 2026-11-12 0310). The repo `rafiqkmateen/network-doctor` is private. Everything is pushed on his go of 2026-11-11 (rows 1 to 3).
- **Built 2026-11-11:** the speed test uses six connections for four seconds (it read 339 down where a website read 918; now about 780 to 800). `share` (address on the real adapter, own network only, code, announces the first phone, advice after a minute). `setup` (asks plan speed, name, which devices are his, weekly check, alerts file, phone page; starts by itself only for a person at a keyboard with no settings file). `config alerts-file on|off`, one tab-split line per alert in `alerts.log`. Linux and Mac readers: sample-text tests everywhere; on GitHub's real machines the adapter was read (Linux wired, 16000 Mbps; Mac wired, link speed 0). Mac drops are "not available" (change list 13). New dependency: `rsc.io/qr` (BSD license; note it for the license row).
- **Not proven:** a phone scanning the code, Wi-Fi live, Linux and Mac facts beyond the adapter, the installed copy (still the old 0.1.0 build with the old speed test; Khadijah's card shows its numbers until the new setup is installed). Setup is built only by GitHub's installer job: Inno Setup is not on his laptop. Windows Application Control blocked some test programs here; I did not work around it, GitHub ran them.
- **Installer (done):** `installer\netdoctor.iss` (Inno Setup 6), `scripts\build-installer.ps1`, `scripts\test-installer.ps1`. Reactive is fixed; scheduled and proactive are tick boxes; silent switches `/VERYSILENT /SUPPRESSMSGBOXES /COMPONENTS="scheduled,proactive"`, `/DELETERESULTS=1` on uninstall. Program Files, data in ProgramData (an installed copy is told by `installed.marker`). Weekly task and log-on alerts task run `netdoctorw.exe`. All four choices passed on GitHub's clean Windows machine. Rafiq saw the window and installed it on his laptop on 2026-11-11, then uninstalled and installed again the proper way: version 0.1.0 is in `C:\Program Files\Network Doctor` with both tasks. Not done: code signing, so strict computers may block it. The finished zip is in `Downloads\Claude finished work\Network Doctor`. The installer does not put the tool on the PATH; an update is a newer setup run over the old copy (no in-tool updater; one would have to be off by default).
- **Khadijah:** her node `network-doctor` now runs the installed copy (looks in her conf file, then `Program Files\Network Doctor`, then the PATH; no build folder). The old `dist\netdoctor.exe` went to the Recycle Bin. To test live: `& 'C:\Program Files\Network Doctor\netdoctor.exe' check`. Her Handover and CLAUDE.md rule her repo; commits there are local until he says push.
- Proven live (wired, 1 Gbps): check, drops, devices, settings, weekly row, watch, JSON, web page, the Khadijah card, the installer window. Not proven: exact per-program bytes (change list 1).
- Windows Application Control blocked an unsigned scratch program earlier; `netdoctor.exe` itself ran.
- Files: `internal\core`, `internal\collect`, `internal\app`, `internal\web`, `internal\store`, `installer`, `scripts`. Plain zips: `powershell -File scripts\package.ps1` into `dist\release`.
