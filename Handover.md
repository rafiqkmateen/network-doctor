# Handover

Purpose: the one job for the next session, and nothing else.
Owner: Rafiq. Version 0.6.0 (0.6.0: wrap of 2026-11-12 0200, repository public, MIT, tasks back, phone proven; 0.5.0: wrap of 2026-11-12; runs from `run\`, speed server refusal handled, GitHub green, matrix holds only rows that need him; 0.4.0: wrap of 2026-11-11 night). Date: 2026-11-12. Finished work is in `CHANGELOG.md`, not here. Keep this page under 1,500 words; prune it at every wrap.

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
| 1 | Phone scan of the https code from `netdoctor share` (press Advanced, then Continue). | Waits for his phone |
| 2 | Run `fix apply name-cache --yes` once the rebuilt copy is allowed by Windows (it was blocked at 0210). | Waits for his "go" |

## State (2026-11-12, laptop clock)

- **Where it runs.** `C:\Users\rafiq\projects\network-doctor\run\netdoctor.exe`, a build of the current code. Backups beside it: `netdoctor-2340.exe` (the build that read 857 down) and `netdoctor-old.exe` (the 1820 build). Git ignores `*.exe`. Khadijah's card runs it through `conf\network-doctor.txt` in her folder. Setup (Program Files) is uninstalled and not used on his laptop.
- **Update the run copy** by building into `run\netdoctor-new.exe` with `go build -trimpath -ldflags '-s -w' -o run\netdoctor-new.exe .\cmd\netdoctor`, running `version` and one check, then moving it over `netdoctor.exe`. If Windows blocks it, put the backup back and tell him.
- **Windows Application Control.** Smart App Control is on and enforcing. It refused the staged `netdoctor.exe` that setup installed (a specific file; the same code built fresh runs, in Program Files style folders and in his user folder). He chose no signing and no turning Smart App Control off. Do not raise it again. Some Go test programs on this laptop are blocked at random: GitHub runs them.
- **Speed server.** Cloudflare's speed server answered 429 "too many requests" for about an hour on 2026-11-12 after repeated tests. The tool now says "not measured" and the wait, holds off between runs (`speed-hold.txt`), reuses a speed under ten minutes old (`speed-last.json`), uses four connections for three seconds, and the weekly check saves no row without a measured speed. Never run repeated live speed tests. Live proof: after the wait, 874 down and 776 up; a second check within a minute reused the speed.
- **GitHub.** Release v0.1.0 published 2026-11-12 about 0212 (plain files for Windows, Mac, Linux and a checksum list; no setup file in the release). PUBLIC since 2026-11-12 about 0200, MIT. History rewritten to the no-reply address and pushed; all four jobs passed. Local commits after the push (docs only) are not pushed. This repo's git email is the no-reply one: never his AOL address. Mac history: he said leave it ("not available").
- **Tasks.** Weekly check (Monday 0900) and alerts (at log-on) run from `run
etdoctorw.exe`.
- **Not proven:** the camera scan of the code itself (the phone did open the page), Wi-Fi live, Linux and Mac facts beyond the adapter, exact per-program bytes (change list 1), the installer on a computer with a strict app rule.
- **Files:** `internal\core`, `internal\collect`, `internal\app`, `internal\web`, `internal\store`, `installer`, `scripts`. Plain zips: `powershell -File scripts\package.ps1` into `dist\release`. Setup is built only by GitHub's installer job.
- **Khadijah's card** for this tool is her job (her Handover): "not measured", the verdict and the rename are committed and pushed; weekly history and alerts are unfinished.
