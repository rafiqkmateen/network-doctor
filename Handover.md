# Handover

Purpose: the one job for the next session, and nothing else.
Owner: Rafiq. Version 0.7.0 (0.7.0: wrap of 2026-11-12 1015, version 0.2.0: Ookla speed source, holds, choices file, repair-ready installer, installed on his laptop; 0.6.0: wrap of 2026-11-12 0200, repository public, MIT, tasks back, phone proven; 0.5.0: wrap of 2026-11-12; runs from `run\`, speed server refusal handled, GitHub green, matrix holds only rows that need him; 0.4.0: wrap of 2026-11-11 night). Date: 2026-11-12. Finished work is in `CHANGELOG.md`, not here. Keep this page under 1,500 words; prune it at every wrap.

## Do this first

1. Read this page, `CLAUDE.md`, `docs\change-list.md` and `docs\test-results.md`.
2. Show the matrix below once, then wait for his word "go" on a row.

## Hard rules

- Plain words, no codes; imperial units; military time (the laptop's clock); short, yes or no first. The full set is in memory.
- Only the word "go" starts work. "Agreed", an answer or a button pick is not a go. A question from him gets an explanation and nothing else: no edits, no matrix tacked on. He may also give a plain order ("build all of it"); then do it and report.
- **The matrix holds only what needs his word.** No watching rows, no parked rows, no rows that are already decided. When he asks for "everything", give the full list once, grouped, and say who moves each row. When he says "not Network Doctor", leave every Network Doctor row out.
- Do the jobs I can do myself (uninstall, find a file, run a check, investigate). Ask him only for what is his: a press on a Windows box, a choice, a go. When he asks "why", investigate and report facts; never guess aloud.
- Test first, then build. A new idea goes on `docs\change-list.md`. Commit finished work at once, local only. Push only to the private repo on his word; make it public only on his word.
- Never work around a Windows security block. Finished files for him go in `C:\Users\rafiq\Downloads\Claude finished work\<job>`.
- Do not start a helper (subagent) without a plan for its cost. One helper used 5.5 million tokens in 8 minutes on 2026-11-12. Do small jobs here; if a helper is needed, give it few steps and watch it.
- Network Doctor stands on its own: no part may need Khadijah or Network Mapper. His three modes: reactive (always on), scheduled and proactive (each offered, explained in plain words, one press is the yes).
- Nothing of Network Doctor goes in Khadijah's folder, and nothing of hers goes here. Her card rows live on her Handover.

## The matrix (only what needs his word)

| # | What | State |
|---|------|-------|
| 1 | Push the local commits (Ookla speed source, holds, choices file, installer, documents) to GitHub, and say whether to publish release 0.2.0 with the setup file. | Only on his word |

## State (2026-11-12, laptop clock)

- **Where it runs.** Installed by the setup file (0.2.0) into `C:\Users\rafiq\projects\network-doctor\run`, with the weekly check and the alerts both chosen: `choices.json`, `unins000.exe`, `installed.marker` and Ookla's tool in `run\ookla` sit there. It is in Apps and features, so the uninstaller works. Results now go to `C:\ProgramData\NetworkDoctor` (the marker says installed). Khadijah's card runs it through `conf\network-doctor.txt`. A backup of the old `run` folder and both tasks is in `Downloads\Claude finished work\network-doctor-run-backup-2026-11-12`.
- **Install or repair on his laptop.** Build with `powershell -File scripts\build-installer.ps1 -Version 0.2.0 -WindowlessTag <tag>` (Inno Setup 6.7.3 is installed under his user folder). Smart App Control blocks some new files at random, so try tags (w2 worked last) until `dist\installer-stage\netdoctor.exe` and `netdoctorw.exe` both start. Then he pastes his usual line for `Downloads\Claude finished work\network-doctor-install\install-and-report.ps1` (clear screen, output into `results.txt`, then done) and I read the file. Running it again is the repair.
- **Windows Application Control.** Smart App Control is on and enforcing. It judges each new file on its own, not by folder (the log is in the Code Integrity event log). He chose no signing and no turning Smart App Control off. Do not raise it again. Blocked Go test programs: build with `go test -c -ldflags=-s -o <file> .\internal\app`, run the file from the package folder, and try `-w` or `-trimpath` if it is blocked.
- **Speed.** Ookla's own command-line tool is used when it is beside the program (`ookla\speedtest.exe`), the `NETDOCTOR_OOKLA` setting or the path; it is never shipped, never accepted for anyone, and the installer offers it only with his tick. It needed secure DNS on his laptop (the Aircove's lookups failed for Ookla's address); the files are in `Downloads\Claude finished work\secure-dns`. Cloudflare's test is the fallback and refused repeated tests once (429). Holds: 2 minutes after Ookla, 10 after Cloudflare; a press inside the hold measures nothing and says when to come back (military time, no colon). Live 2026-11-12: Ookla 807 and 930 down, 749 and 617 up; Cloudflare 789 down, 521 up.
- **GitHub.** Release v0.1.0 published 2026-11-12 about 0212 (plain files for Windows, Mac, Linux and a checksum list; no setup file in the release). PUBLIC since 2026-11-12 about 0200, MIT. History rewritten to the no-reply address and pushed; all four jobs passed. Local commits after the push (about ten, program, installer and documents) are not pushed. This repo's git email is the no-reply one: never his AOL address. Mac history: he said leave it ("not available").
- **Tasks.** Weekly check (Monday 0900) and alerts (at log-on) run `run\netdoctorw.exe`; the installer recreates them for chosen parts and deletes them for parts not chosen.
- **Not proven:** the camera scan of the code itself (the phone did open the page), Wi-Fi live, Linux and Mac facts beyond the adapter, exact per-program bytes (change list 1), the Ookla page of the setup window (the tick, download and unpack) on a screen; the clean-machine install, repair and uninstall run on GitHub.
- **Files:** `internal\core`, `internal\collect`, `internal\app`, `internal\web`, `internal\store`, `installer`, `scripts`. Plain zips: `powershell -File scripts\package.ps1` into `dist\release`. Setup is built only by GitHub's installer job.
- **Khadijah's card** for this tool is her job (her Handover): it now reads `choices.json` and shows only the chosen parts, with labels, the 2000 note under the verdict, the tool line and the Too soon box.
