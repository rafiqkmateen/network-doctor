# Handover

Purpose: the one job for the next session, and nothing else.
Owner: Rafiq. Version 0.1.4 (0.1.4: installer window seen by Rafiq, row removed; 0.1.3: matrix reordered so the pictures and the demo video come after the card changes; 0.1.2: Windows installer built and passing on GitHub; Khadijah node built, uncommitted; 0.1.1: stage 1 handover). Date: 2026-11-11. Finished work is in `CHANGELOG.md`, not here. Keep this page under 1,500 words; prune it at every wrap.

## Do this first

1. Read this page, `CLAUDE.md`, `docs\change-list.md` and `docs\test-results.md`.
2. Show the matrix below once, then wait for his word "go" on a row.

## Hard rules

- Plain words, no codes; imperial units; military time; short, yes or no first; one question at a time, with two buttons (the `wrap_up` box), never a bare list. The full set is in memory.
- Only the word "go" starts work. "Agreed", an answer or a button pick is not a go. Read-only checks to answer a question are fine.
- Test first, then build. A new idea goes on `docs\change-list.md`. Commit finished work at once. Push only to the private repo, and make it public only on his word.
- Never work around a Windows security block. Finished files for him go in `C:\Users\rafiq\Downloads\Claude finished work\Network Doctor`.
- The demo video is handled in another chat. Do not do it here.
- Network Doctor stands on its own: no part may need Khadijah or Network Mapper. His three modes: reactive (always on), scheduled and proactive (each offered, explained in plain words, one press is the yes).

## The matrix

| # | What | Industry standard | State |
|---|------|-------------------|-------|
| 1 | Remove the NET UP and NET DOWN readout from Khadijah's top strip, with tests, requirements and guide updated. | Record the change, then change the tests and documents with the code. | Waits for his "go". |
| 2 | Add a Good, Watch or Bad verdict and one plain sentence to each line of the Network Doctor card; rename "Delay to the internet" to "Reply time to the internet"; keep the card up 3 minutes. | Tools say what a reading means, not only the number. | Waits for his "go". |
| 3 | Phone sharing built into the tool, and a first-run helper that asks "are these your devices?" and offers the parts (change list 9 and 10). Comes before the weekly history row. | Tools guide the user themselves. | Waits for his "go". |
| 4 | Weekly history and alerts in the Khadijah card, and Network Doctor writing alerts to a file only when switched on. | Alerts reach the user without the user asking. | Waits for his "go". |
| 5 | Finish the Khadijah node: commit it, retake the guide picture (it shows the old "7 unknown"). Local commit only. | Tests first, then build, then check against the requirement. | Waits for his "go". Comes after rows 2 to 5, so the pictures show the finished cards. |
| 6 | Real Linux readers, then real Mac readers. | Test on the real system. | Waits for his "go". |
| 7 | Make the repository public: license (not yet chosen), no-reply email, front page, first release. Code-signing for the Windows files (change list 2) before customers get the installer. | Public code needs a license and no personal data. | Waits for his license answer. |
| 8 | Stage 2: apply a fix with approval. | Least privilege, undo, audit log. | After stage 1 is accepted. |

## State (2026-11-11)

- Stage 1 is built (Go 1.27.4). `go test ./...` passes; GitHub passes on Windows, Mac and Linux, and a fourth job tests the installer. The repo `rafiqkmateen/network-doctor` is private and everything is pushed.
- **Installer (change list 11, done):** `installer\netdoctor.iss` (Inno Setup 6), `scripts\build-installer.ps1`, `scripts\test-installer.ps1`. Reactive is fixed; scheduled and proactive are tick boxes; silent switches `/VERYSILENT /SUPPRESSMSGBOXES /COMPONENTS="scheduled,proactive"`, `/DELETERESULTS=1` on uninstall. Program Files, data in ProgramData (an installed copy is told by `installed.marker`; the unzipped file stays per person). Weekly task and log-on alerts task run `netdoctorw.exe` (no console window). All four choices installed and uninstalled clean on GitHub's Windows machine (run 43131744806). Not seen: the window itself. Not done: code signing, so strict computers may block it. The finished zip is in `Downloads\Claude finished work\Network Doctor`.
- **Khadijah node (built in `C:\Users\rafiq\khadijah`, not committed):** node `network-doctor`, `services\network_doctor.py`, `routes\network_doctor.py`, card in `frontend\cards.js`, logo, guide page 20 and the features row, change list 91, requirements R-NDR-01 to 08, 16 tests passing. It subtracts devices Network Mapper trusts and the router, so the unknown count agrees with Mapper. Her check run: all pass except the starter check, which also fails on a clean tree (not ours). Her own Handover and CLAUDE.md rule that repo; a commit there is local until he says push.
- Proven live on this laptop (wired, 1 Gbps): check, drops, devices, settings, weekly row, watch, JSON, web page, and the Khadijah card. Not proven: Wi-Fi live, Mac and Linux readers (not built), phone, exact per-program bytes (change list 1).
- Windows Application Control blocked an unsigned scratch program earlier; `netdoctor.exe` itself ran.
- Files: `internal\core`, `internal\collect`, `internal\app`, `internal\web`, `internal\store`, `installer`, `scripts`. Plain zips: `powershell -File scripts\package.ps1` into `dist\release`.
