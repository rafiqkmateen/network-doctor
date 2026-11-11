# Handover

Purpose: the one job for the next session, and nothing else.
Owner: Rafiq. Version 0.1.1. Date: 2026-11-11. Finished work is in `CHANGELOG.md`, not here. Keep this page under 1,500 words; prune it at every wrap.

## Do this first

1. Read this page, `CLAUDE.md`, `docs\change-list.md` and `docs\test-results.md`.
2. Show the matrix below once, then wait for his word "go" on a row.

## Hard rules

- Plain words, no codes; imperial units; military time; short, yes or no first; one question at a time. The full set is in memory.
- Only the word "go" starts work. Read-only checks to answer a question are fine.
- Test first, then build. A new idea goes on `docs\change-list.md`. Commit finished work at once. Push only to the private repo, and make it public only on his word.
- Never work around a Windows security block. Finished files for him go in `C:\Users\rafiq\Downloads\Claude finished work\Network Doctor`.
- The demo video is handled in another chat. Do not do it here.

## The matrix

| # | What | Industry standard | State |
|---|------|-------------------|-------|
| 1 | Add the Network Doctor node to Khadijah: logo, guide page, features row, picture. Work is done in `C:\Users\rafiq\khadijah`; see "Khadijah node" below. | Tests first, then build, then check against the requirement. | Waits for his "go". |
| 2 | Phone sharing built into the tool, and a first-run helper (change list 9 and 10). | Tools guide the user themselves. | Waits for his "go". |
| 3 | Real Linux readers, then real Mac readers (adapter, log, devices, settings). GitHub's machines can run them. | Test on the real system. | Waits for his "go". |
| 4 | Make the repository public: pick a license (he has not answered MIT), switch commits to the GitHub no-reply email, front page with screenshots, first release. | Public code needs a license and no personal data. | Waits for his license answer. |
| 5 | Stage 2: apply a fix with approval. | Least privilege, undo, audit log. | After stage 1 is accepted. |

## State (2026-11-11 2338)

- Stage 1 is built (Go 1.27.4, installed by winget). `go test ./...` passes; the GitHub run passed on Windows, Mac and Linux. The repo is `rafiqkmateen/network-doctor`, private, nothing unsent.
- Proven live on this Windows laptop (wired, 1 Gbps): the check, drops, devices, settings, weekly row, watch, JSON, web page in the built-in browser. Not proven: Wi-Fi live (card was disconnected), Mac and Linux readers (not built), phone (not tried), exact per-program bytes (needs administrator rights, change list 1).
- Found live and fixed: sub-second flaps, own computer flagged, "power saving off" when Windows would not say, per-program counters useless.
- Windows Application Control blocked an unsigned scratch program; `netdoctor.exe` itself ran. A code-signing certificate is change list 2.
- Files: `internal\core` (checks and words), `internal\collect` (readers and scripts), `internal\app` (commands, watch), `internal\web` (page), `internal\store` (files). Release files: `powershell -File scripts\package.ps1` into `dist\release`.

## Khadijah node (row 1)

Khadijah is at `C:\Users\rafiq\khadijah`; her own `Handover.md` and `CLAUDE.md` rule that repo. Plan: node `network-doctor` in group System Subsystems beside `net-mapper`, type like `netmap`; five places to touch: `frontend\graph-nodes.js`, a logo in `frontend\logos`, panel code in `frontend\cards.js`, `features.py`, `docs\guide\images\picture-map.json`; then the guide page, the what-she-can-do row, changelog and Handover there. The panel runs `netdoctor check --json` and reads it. Logo idea: a network plug with a stethoscope, red, badge DOCTOR, like the Network Mapper logo. Her checks: `scripts\python\check_all.py`, `check_docs.py`, `check_features.py`. Start her only as her Handover says. A Khadijah commit is local only until he says push.
