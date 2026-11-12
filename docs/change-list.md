# Change list

Purpose: ideas and changes waiting for Rafiq's decision. Nothing here is in the build.
Owner: Rafiq. Version 0.6.0 (0.6.0: row 13 added; 0.5.0: rows 3, 9 and 10 agreed and in build; 0.4.0: row 12 added and built; 0.3.0: rows 6 and 11 brought up to date, 2026-11-11; 0.2.0: row 11 added and agreed; 0.1.0: first rows).

| # | Change | Why | State |
|---|---|---|---|
| 1 | Optional administrator mode that reads exact per-program upload and download (Windows kernel network trace) | Without administrator rights Windows gives no per-program bytes. Requirement ND-20 is only partly met today. | Waits for Rafiq. |
| 2 | Code-signing certificate for the Windows file | Strict Windows app control can block an unsigned file. | Waits for Rafiq. |
| 3 | Mac and Linux collectors (adapter, log, devices, settings) | Today those two systems get ping, speed and DNS only. | Agreed: Rafiq said "1 go" 2026-11-11 (requirements ND-160 to ND-172). In build. |
| 4 | Tray icon for the laptop | Proactive mode shows alerts on screen today through a Windows pop-up. | Not started. |
| 5 | Stage 2: apply a fix with approval (ND-90 to ND-96) | Agreed as the second stage. | After stage 1 is accepted. |
| 6 | Khadijah as an alert place, and the Khadijah node | Agreed. | Node built and committed in Khadijah (local); it runs the installed copy. Weekly history card and alerts file wait for Rafiq. |
| 7 | Larger list of device makers | The built-in list is short; unknown makers show as unknown. A `makers` file can extend it in future. | Not started. |
| 8 | Native phone app | Agreed as later. | Later. |
| 9 | Phone sharing built into the tool: one command (`share`) that finds this computer's address, picks the allowed network range by itself, starts the page, and shows the exact address and a scannable code on screen. It also tests that the page can be reached and says what to click if the firewall or a guest network or VPN is in the way. | The phone steps lived only in the guide. People should not need the guide. Raised by Rafiq 2026-11-11. | Agreed: "1 go" 2026-11-11 (ND-130 to ND-134). In build. |
| 10 | First-run helper: on first start the tool asks for the plan speed and a name, offers the weekly check and the phone page, and explains each in plain words. Same idea everywhere: the tool tells you what to do next, not the guide. | Same reason as 9. | Agreed: "1 go" 2026-11-11 (ND-140 to ND-143). In build. |
| 11 | Windows installer: one `setup.exe` in a zip. Reactive (on-demand check) always installs; the person picks weekly check (scheduled) and alerts (proactive). Window for one person, silent switches for IT, uninstaller in Apps and features. Built with Inno Setup, installed in Program Files, results shared in ProgramData, alerts as a log-on task. | Customers install it on their own computers and get only the parts they pick. Raised by Rafiq 2026-11-11; plan agreed the same day. | Done 2026-11-11: built, passed on GitHub's clean Windows machine, window seen and installed by Rafiq. Windows only; Mac and Linux wait for their readers. |
| 12 | Speed test uses several connections at once for a few seconds, not one file on one connection | Found 2026-11-11: on a line the website measured at 918 down and 641 up, the tool read 339 and 111. One connection and a half-second file cannot fill a gigabit line. Same trick as the websites; no new program. | Agreed: Rafiq said "go" 2026-11-11. Built with its tests. |
| 13 | Mac connection history (drops over seven days) | The Mac log takes minutes to search and its wording changes between versions, so the Mac reader says "not available" for drops. Found while building change list 3. | Waits for Rafiq. |
