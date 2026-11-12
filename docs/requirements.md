# Requirements

Purpose: the fixed list of what Network Doctor must do. Each requirement has one test in `test-cases.md`.
Owner: Rafiq. Version 0.3.0 (0.3.0: ND-130 to ND-172 added by change list 3, 9 and 10; 0.2.0: ND-110 to ND-120 added by change list 11, agreed by Rafiq 2026-11-11; 0.1.0: draft). Date: 2026-11-11.

Form: each requirement is one "shall" sentence that can be tested. A new idea goes on the change list, not here.

## Ground rules

- ND-01 The tool shall only read and report in stage 1; it shall not change any setting, driver, device or program.
- ND-02 The tool shall run with no install, as a single executable per system (Windows, Mac, Linux, ARM Linux), also offered as a zip with a one-page guide.
- ND-03 The tool shall work offline except for the speed test and internet checks.
- ND-04 The tool shall keep all saved data on the local machine and shall send nothing anywhere unless the user turns it on.
- ND-05 All text shown to people shall be plain English at a 6th to 8th grade level.

## Checks

- ND-10 Diagnose: the tool shall detect whether the active connection is wired or Wi-Fi.
- ND-11 Diagnose: the tool shall measure speed and delay to the router and to the internet, and packet loss.
- ND-12 Diagnose: the tool shall name the most likely cause (this computer, cable or signal, router, or provider) and what to check first.
- ND-20 Bandwidth: the tool shall list the programs using the connection, ranked, with upload and download.
- ND-21 Bandwidth: the tool shall recommend which activities could be paused, and shall not stop any.
- ND-30 Drops: the tool shall read the last seven days of connection history and report the number of drops, the time of each and the length.
- ND-31 Drops: the tool shall point out patterns (time of day, power-saving, driver).
- ND-40 Link health, wired: the tool shall report link speed, duplex and error counts.
- ND-41 Link health, Wi-Fi: the tool shall report signal strength, band, channel and nearby networks on the same channel, with advice.
- ND-50 Devices: the tool shall list devices on the network with name, maker, address and hardware address, and flag any not marked known.
- ND-51 Devices: the tool shall never guess, store or log a router password.
- ND-60 Settings: the tool shall review DNS, adapter power-saving, VPN, proxies, adapter and driver version and configuration errors, and explain each finding and the effect of changing it.

## Modes

- ND-70 Reactive: the user shall be able to run a full check on demand.
- ND-71 Scheduled: the user shall be able to turn on a weekly check (default Monday 0900) that records speed, delay, date, time and subscribed speed to a spreadsheet file.
- ND-72 Scheduled: if download stays under half the subscribed speed for three consecutive weekly checks, the tool shall prepare a complaint email as a draft and shall never send it.
- ND-73 Proactive: the tool shall notify the user when the link drops, when speed stays below plan, or when a new unknown device appears.
- ND-74 Proactive: each alert type shall be switchable, shall honor quiet hours, and shall give one alert per outage.
- ND-75 Proactive: alerts shall go only where the user chooses; by default nowhere outside the machine.

## Interfaces

- ND-80 The tool shall have a command-line mode with no screen, JSON output and clear exit codes, for servers.
- ND-81 The tool shall serve a local web page for people and phones, reachable only from the local machine or networks the user allows.

## Stage 2 (apply with approval)

- ND-90 The tool shall apply a fix only after the user approves that single fix.
- ND-91 Before applying, the tool shall show the before and after values.
- ND-92 The tool shall save the old value and undo the change with one action.
- ND-93 The tool shall write every applied change to a log.
- ND-94 The tool shall request administrator rights only at the moment of approval.
- ND-95 The tool shall never apply driver changes, security-protection changes, or any change that could cut off the connection in use.
- ND-96 On an unattended server, apply shall be off by default and switchable only per machine by the user.

## Installer (Windows, change list 11)

- ND-110 A Windows installer (`setup.exe`, offered in a zip with the guide) shall install Network Doctor without relying on any program, setting or file already on the computer.
- ND-111 The installer shall always install the on-demand check (reactive), and the person shall not be able to clear it.
- ND-112 The installer shall ask whether to add the weekly check (scheduled) and the alerts (proactive), each with a plain sentence saying what it does, and shall install only what is chosen.
- ND-113 Choosing neither, either one or both shall install exactly the on-demand check plus the chosen parts and nothing else.
- ND-114 The installer shall also run with no window, with switches that carry the same choices, for IT.
- ND-115 The installer shall put the program in Program Files, add an entry to Apps and features, and ask for administrator rights once.
- ND-116 An installed copy shall save its results, known devices and alerts in ProgramData; an unzipped copy shall keep saving per person.
- ND-117 The weekly check shall be a weekly task, and the alerts a task that starts when someone logs on and shows alerts on their screen; neither shall exist unless chosen.
- ND-118 The uninstaller (in Apps and features, in the program folder, and with a silent switch) shall undo exactly what the install did, and shall ask whether to keep the saved results, with keep as the default.
- ND-119 The installer and uninstaller shall send nothing off the computer.
- ND-120 The installer shall be tested for all four choices, install then uninstall, on a Windows computer that has none of the developer's setup.

## Phone sharing, first-run helper, alerts file, Mac and Linux (change list 3, 9, 10; agreed by Rafiq 2026-11-11)

- ND-130 A `share` command shall find this computer's address on the real network adapter (not a VPN), start the web page for that one network only, and print the address.
- ND-131 `share` shall also print a code on the screen that a phone camera can scan to open that address.
- ND-132 `share` shall wait for the first request from another device in the allowed range and say when it arrives; if none arrives within a minute it shall give plain advice from read-only facts (the Windows allow box, a public network, a VPN, a guest network).
- ND-133 `share` shall allow only the adapter's own network, and shall never change a firewall rule or any other setting.
- ND-134 If no network address can be found, `share` shall say so in plain words and shall not start.
- ND-140 A `setup` command (the first-run helper) shall ask for the plan speed and a name, show the devices found and ask which are the person's, and offer the weekly check, the alerts file and the phone page, one yes or no each.
- ND-141 The helper shall start by itself only when a person at a keyboard runs `netdoctor` with no command and no settings file exists; never for `check`, `weekly`, `watch`, `serve`, JSON, the no-window build, or input that is not a keyboard.
- ND-142 Each offer shall say in plain words what it does, and each answer shall default to no.
- ND-143 The helper shall change nothing until the person says yes, and shall save only the tool's own settings and lists.
- ND-150 One command (`config alerts-file on` or `off`) shall switch the alerts file on or off without changing the other places alerts go.
- ND-151 When the alerts file is on, each alert shall be one line of time, kind and message, in a format written in the guide; when it is off nothing shall be written.
- ND-152 The alerts file shall be saved in the same folder as the tool's other results.
- ND-160 On Linux the tool shall read, without changing anything: the adapter (wired or Wi-Fi, link speed, duplex, errors, gateway, address), Wi-Fi signal, band and channel, line use and busiest programs, seven days of connection history, devices, and the six settings areas.
- ND-161 Where Linux cannot give a fact without administrator rights, the tool shall say so in plain words and shall not guess.
- ND-162 The reading of the system's own text on Linux shall be tested on sample text on every system the tests run on.
- ND-163 Every command the Linux and Mac readers run shall be read-only, checked by a test.
- ND-164 On GitHub's real Linux and Mac machines, a run of the built program shall read the adapter and say which kind it is.
- ND-170 On Mac the tool shall read the same facts as ND-160, without changing anything.
- ND-171 Where Mac cannot give a fact without administrator rights, the tool shall say so and not guess.
- ND-172 The reading of the system's own text on Mac shall be tested on sample text on every system the tests run on.

## Release

- ND-100 Each release shall include checksums and a signature.
- ND-101 Automated tests shall run on Windows, Mac and Linux for every change.

## Later (change list, not in this build)

- A Khadijah node that calls the tool and reads its results.
- A smaller native phone app.
