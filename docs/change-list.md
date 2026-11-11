# Change list

Purpose: ideas and changes waiting for Rafiq's decision. Nothing here is in the build.
Owner: Rafiq. Version 0.1.0.

| # | Change | Why | State |
|---|---|---|---|
| 1 | Optional administrator mode that reads exact per-program upload and download (Windows kernel network trace) | Without administrator rights Windows gives no per-program bytes. Requirement ND-20 is only partly met today. | Waits for Rafiq. |
| 2 | Code-signing certificate for the Windows file | Strict Windows app control can block an unsigned file. | Waits for Rafiq. |
| 3 | Mac and Linux collectors (adapter, log, devices, settings) | Today those two systems get ping, speed and DNS only. | Not started. |
| 4 | Tray icon for the laptop | Proactive mode shows alerts on screen today through a Windows pop-up. | Not started. |
| 5 | Stage 2: apply a fix with approval (ND-90 to ND-96) | Agreed as the second stage. | After stage 1 is accepted. |
| 6 | Khadijah as an alert place, and the Khadijah node | Agreed. | Node in progress. |
| 7 | Larger list of device makers | The built-in list is short; unknown makers show as unknown. A `makers` file can extend it in future. | Not started. |
| 8 | Native phone app | Agreed as later. | Later. |
| 9 | Phone sharing built into the tool: one command (`share`) that finds this computer's address, picks the allowed network range by itself, starts the page, and shows the exact address and a scannable code on screen. It also tests that the page can be reached and says what to click if the firewall or a guest network or VPN is in the way. | The phone steps lived only in the guide. People should not need the guide. Raised by Rafiq 2026-11-11. | Waits for Rafiq's go. |
| 10 | First-run helper: on first start the tool asks for the plan speed and a name, offers the weekly check and the phone page, and explains each in plain words. Same idea everywhere: the tool tells you what to do next, not the guide. | Same reason as 9. | Waits for Rafiq's go. |
