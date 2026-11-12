# Change list

Purpose: ideas and changes waiting for Rafiq's decision. Nothing here is in the build.
Owner: Rafiq. Version 0.18.0 (0.18.0: built rows 3, 5, 6, 9 to 12, 14, 15 and 17 to 20 moved to `docs\archive\change-list-done.md`; 0.17.0: rows 18 to 20 built; 0.16.0: row 20 holds decided, 10 minutes and 2 minutes; 0.15.0: row 20 proven live; 0.14.0: row 20 added, Ookla's command-line tool; 0.13.0: row 19 gains the folder choice; 0.12.0: row 19 added, a choices file; 0.11.0: row 18 reworded to a hold with a come-back time; 0.10.0: row 18 added; 0.9.0: row 15 built; 0.8.0: rows 14 and 15 started on Rafiq's "go"; 0.7.0: rows 14 and 15 added, speed server refusal; 0.6.0: row 13 added; 0.5.0: rows 3, 9 and 10 agreed and in build; 0.4.0: row 12 added and built; 0.3.0: rows 6 and 11 brought up to date, 2026-11-11; 0.2.0: row 11 added and agreed; 0.1.0: first rows).

| # | Change | Why | State |
|---|---|---|---|
| 1 | Optional administrator mode that reads exact per-program upload and download (Windows kernel network trace) | Without administrator rights Windows gives no per-program bytes. Requirement ND-20 is only partly met today. | Waits for Rafiq. |
| 2 | Code-signing certificate for the Windows file | Strict Windows app control can block an unsigned file. | Waits for Rafiq. |
| 4 | Tray icon for the laptop | Proactive mode shows alerts on screen today through a Windows pop-up. | Not started. |
| 7 | Larger list of device makers | The built-in list is short; unknown makers show as unknown. A `makers` file can extend it in future. | Not started. |
| 8 | Native phone app | Agreed as later. | Later. |
| 13 | Mac connection history (drops over seven days) | The Mac log takes minutes to search and its wording changes between versions, so the Mac reader says "not available" for drops. Found while building change list 3. | Waits for Rafiq. |
| 16 | Serve the phone page over HTTPS (`share` and `serve`) | Rafiq, 2026-11-12: "we can make that page HTTPS". Today the page is plain HTTP on the local network only. A certificate for a bare address is not possible from a public authority, so the choices are a self-made certificate (phones warn once) or a local name with a certificate the user installs. Rafiq said "go" 2026-11-12 on the self-made certificate. | Built with tests (ND-190 to ND-192). Phone scan still to prove. |
