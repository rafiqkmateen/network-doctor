# Handover

Purpose: the one job for the next session, and nothing else.
Owner: Rafiq. Version 0.2.1 (0.2.1: code signing made its own row and rows put in dependency order; 0.2.0: wrap of 2026-11-11, pruned; installer seen and installed on his laptop, Khadijah now uses the installed copy, Khadijah-side rows moved to her Handover; 0.1.x: installer and stage 1). Date: 2026-11-11. Finished work is in `CHANGELOG.md`, not here. Keep this page under 1,500 words; prune it at every wrap.

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
| 1 | Phone sharing built into the tool: one command finds the address, starts the page, shows a code to scan, and tests that the phone can reach it. Plus a first-run helper that asks "are these your devices?" and offers the parts. | Tools guide the user themselves. | Waits for his "go". Starts rows 1 to 3 in one pass. |
| 2 | Network Doctor writes alerts to a file, only when switched on. | Alerts reach the user without the user asking. | Part of the same "go" as row 1. |
| 3 | Real Linux readers, then real Mac readers, tested on GitHub's real Linux and Mac machines. | Test on the real system. | Part of the same "go" as row 1. |
| 4 | Code signing: a paid certificate that proves the Windows file really comes from you. Without it, a strict Windows computer can block the file. Your choice and a purchase. | Sign what you ship. | Waits for his answer. Comes before going public. |
| 5 | Pick a license: the rule that says what other people may do with the code. | Public code needs a license. | Waits for his answer. Comes before going public. |
| 6 | Make the repository public: a no-reply email, a front page, a first release. | Public code needs no personal data. | Waits for his "go", after rows 4 and 5. |
| 7 | Stage 2: let the tool fix a problem itself, only with his yes on each fix, a before and after view, one-press undo and a change log. | Least privilege, undo, audit log. | After stage 1 is accepted. |

## State (2026-11-11)

- Stage 1 is built (Go 1.27.4). `go test ./...` passes; GitHub passes on Windows, Mac and Linux, and a fourth job tests the installer. The repo `rafiqkmateen/network-doctor` is private. Two docs commits are local only (pushed: everything before them).
- **Installer (done):** `installer\netdoctor.iss` (Inno Setup 6), `scripts\build-installer.ps1`, `scripts\test-installer.ps1`. Reactive is fixed; scheduled and proactive are tick boxes; silent switches `/VERYSILENT /SUPPRESSMSGBOXES /COMPONENTS="scheduled,proactive"`, `/DELETERESULTS=1` on uninstall. Program Files, data in ProgramData (an installed copy is told by `installed.marker`). Weekly task and log-on alerts task run `netdoctorw.exe`. All four choices passed on GitHub's clean Windows machine. Rafiq saw the window and installed it on his laptop on 2026-11-11, then uninstalled and installed again the proper way: version 0.1.0 is in `C:\Program Files\Network Doctor` with both tasks. Not done: code signing, so strict computers may block it. The finished zip is in `Downloads\Claude finished work\Network Doctor`. The installer does not put the tool on the PATH; an update is a newer setup run over the old copy (no in-tool updater; one would have to be off by default).
- **Khadijah:** her node `network-doctor` now runs the installed copy (looks in her conf file, then `Program Files\Network Doctor`, then the PATH; no build folder). The old `dist\netdoctor.exe` went to the Recycle Bin. To test live: `& 'C:\Program Files\Network Doctor\netdoctor.exe' check`. Her Handover and CLAUDE.md rule her repo; commits there are local until he says push.
- Proven live (wired, 1 Gbps): check, drops, devices, settings, weekly row, watch, JSON, web page, the Khadijah card, the installer window. Not proven: Wi-Fi live, Mac and Linux readers (not built), phone, exact per-program bytes (change list 1).
- Windows Application Control blocked an unsigned scratch program earlier; `netdoctor.exe` itself ran.
- Files: `internal\core`, `internal\collect`, `internal\app`, `internal\web`, `internal\store`, `installer`, `scripts`. Plain zips: `powershell -File scripts\package.ps1` into `dist\release`.
